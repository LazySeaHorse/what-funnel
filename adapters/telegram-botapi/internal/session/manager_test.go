package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/whatfunnel/whatfunnel/adapters/telegram-botapi/internal/botapi"
	wfcrypto "github.com/whatfunnel/whatfunnel/packages/go-common/crypto"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

type recordingPublisher struct {
	mu     sync.Mutex
	events []messaging.Event
}

func (p *recordingPublisher) Publish(_ context.Context, event messaging.Event) error {
	p.mu.Lock()
	p.events = append(p.events, event)
	p.mu.Unlock()
	return nil
}

func (p *recordingPublisher) count(kind messaging.EventKind) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	count := 0
	for _, event := range p.events {
		if event.Kind == kind {
			count++
		}
	}
	return count
}

type fakeTelegram struct {
	mu        sync.Mutex
	bots      map[string]botapi.User
	updates   map[string][]botapi.Update
	offsets   map[string][]int64
	sendCalls map[string]int
	methods   map[string]int
	failSend  map[string]bool
}

func newFakeTelegram(t *testing.T) *fakeTelegram {
	t.Helper()
	fake := &fakeTelegram{bots: make(map[string]botapi.User), updates: make(map[string][]botapi.Update), offsets: make(map[string][]int64), sendCalls: make(map[string]int), methods: make(map[string]int), failSend: make(map[string]bool)}
	return fake
}

func (f *fakeTelegram) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	recorder := httptest.NewRecorder()
	f.serveHTTP(recorder, request)
	return recorder.Result(), nil
}

func (f *fakeTelegram) serveHTTP(w http.ResponseWriter, request *http.Request) {
	path := strings.TrimPrefix(request.URL.Path, "/bot")
	token, method, ok := strings.Cut(path, "/")
	if !ok {
		http.NotFound(w, request)
		return
	}
	var params map[string]json.RawMessage
	_ = json.NewDecoder(request.Body).Decode(&params)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.methods[token+":"+method]++
	switch method {
	case "getMe":
		f.respond(w, f.bots[token])
	case "deleteWebhook":
		f.respond(w, true)
	case "getUpdates":
		var offset int64
		_ = json.Unmarshal(params["offset"], &offset)
		f.offsets[token] = append(f.offsets[token], offset)
		result := make([]botapi.Update, 0)
		for _, update := range f.updates[token] {
			if update.UpdateID >= offset {
				result = append(result, update)
			}
		}
		f.respond(w, result)
	case "sendMessage":
		f.sendCalls[token]++
		if f.failSend[token] {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked by the user"}`)
			return
		}
		f.respond(w, botapi.Message{MessageID: 99, Date: 200, Chat: botapi.Chat{ID: 42, Type: "private"}})
	case "sendPhoto", "sendVideo", "sendAudio", "sendVoice", "sendDocument", "editMessageText", "editMessageCaption":
		f.respond(w, botapi.Message{MessageID: 99, Date: 200, Chat: botapi.Chat{ID: 42, Type: "private"}})
	default:
		f.respond(w, true)
	}
}

type staticMediaSource struct{}

func (staticMediaSource) Fetch(context.Context, string) (MediaFile, error) {
	return MediaFile{Data: []byte("document"), MIMEType: "application/pdf", Filename: "guide.pdf"}, nil
}

func (f *fakeTelegram) respond(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
}

func (f *fakeTelegram) addBot(token string, id int64, username string) {
	f.mu.Lock()
	f.bots[token] = botapi.User{ID: id, IsBot: true, FirstName: username, Username: username}
	f.mu.Unlock()
}

func newTestManager(t *testing.T, databasePath string, fake *fakeTelegram, publisher EventPublisher) *Manager {
	t.Helper()
	cipher, err := wfcrypto.NewCipherFromBytes([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	api, err := botapi.New("https://api.telegram.test", &http.Client{Transport: fake, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(t.Context(), databasePath, cipher, api, publisher, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	return manager
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not satisfied")
}

func TestManagerEncryptsCredentialsAndDeduplicatesUpdates(t *testing.T) {
	fake := newFakeTelegram(t)
	const token = "101:very-secret-token"
	fake.addBot(token, 101, "sales_bot")
	update := botapi.Update{UpdateID: 7, Message: &botapi.Message{MessageID: 8, Date: 100, Chat: botapi.Chat{ID: 42, Type: "private"}, From: &botapi.User{ID: 42, FirstName: "Ada"}, Text: "hello"}}
	fake.updates[token] = []botapi.Update{update, update}
	publisher := &recordingPublisher{}
	manager := newTestManager(t, filepath.Join(t.TempDir(), "telegram.db"), fake, publisher)
	snapshot, err := manager.Create(t.Context(), "channel-a", token)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.RemoteAccountID != "@sales_bot" {
		t.Fatalf("remote account = %q", snapshot.RemoteAccountID)
	}
	waitFor(t, func() bool { return publisher.count(messaging.EventMessageCreated) == 1 })
	var encrypted string
	if err := manager.db.QueryRow(`SELECT encrypted_token FROM telegram_sessions WHERE channel_id = ?`, "channel-a").Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if encrypted == token || strings.Contains(encrypted, token) {
		t.Fatal("bot token was stored in plaintext")
	}
	var offset int64
	if err := manager.db.QueryRow(`SELECT update_offset FROM telegram_sessions WHERE channel_id = ?`, "channel-a").Scan(&offset); err != nil {
		t.Fatal(err)
	}
	if offset != 8 {
		t.Fatalf("offset = %d, want 8", offset)
	}
}

func TestLogoutRemovesCredentialAndProviderSessionState(t *testing.T) {
	fake := newFakeTelegram(t)
	fake.addBot("151:logout-secret", 151, "logout_bot")
	manager := newTestManager(t, filepath.Join(t.TempDir(), "telegram.db"), fake, &recordingPublisher{})
	if _, err := manager.Create(t.Context(), "channel-logout", "151:logout-secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.db.Exec(`INSERT INTO adapter_commands (command_id, channel_id, provider_message_id) VALUES ('command-logout', 'channel-logout', '1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.db.Exec(`INSERT INTO adapter_event_outbox (event_id, channel_id, payload, available_at) VALUES ('event-logout', 'channel-logout', '{}', ?)`, time.Now().Add(time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Logout(t.Context(), "channel-logout"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"telegram_sessions", "adapter_commands", "adapter_event_outbox"} {
		var count int
		if err := manager.db.QueryRow(`SELECT count(*) FROM ` + table + ` WHERE channel_id = 'channel-logout'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("%s retained %d rows", table, count)
		}
	}
	if _, err := manager.Snapshot("channel-logout"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Snapshot() error = %v, want ErrNotFound", err)
	}
}

func TestManagerRestoresOffsetsAndIsolatesAccounts(t *testing.T) {
	fake := newFakeTelegram(t)
	fake.addBot("201:alpha", 201, "alpha_bot")
	fake.addBot("202:beta", 202, "beta_bot")
	fake.updates["201:alpha"] = []botapi.Update{{UpdateID: 10, Message: &botapi.Message{MessageID: 1, Date: 100, Chat: botapi.Chat{ID: 41, Type: "private"}, Text: "alpha"}}}
	fake.updates["202:beta"] = []botapi.Update{{UpdateID: 20, Message: &botapi.Message{MessageID: 1, Date: 100, Chat: botapi.Chat{ID: 42, Type: "private"}, Text: "beta"}}}
	databasePath := filepath.Join(t.TempDir(), "telegram.db")
	firstPublisher := &recordingPublisher{}
	manager := newTestManager(t, databasePath, fake, firstPublisher)
	if _, err := manager.Create(t.Context(), "channel-alpha", "201:alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Create(t.Context(), "channel-beta", "202:beta"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return firstPublisher.count(messaging.EventMessageCreated) == 2 })
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}

	secondPublisher := &recordingPublisher{}
	restored := newTestManager(t, databasePath, fake, secondPublisher)
	waitFor(t, func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		return containsOffset(fake.offsets["201:alpha"], 11) && containsOffset(fake.offsets["202:beta"], 21)
	})
	if _, err := restored.Snapshot("channel-alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := restored.Snapshot("channel-beta"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if secondPublisher.count(messaging.EventMessageCreated) != 0 {
		t.Fatal("restored manager republished processed updates")
	}
}

// TestTelegramOffsetPersistence_MidStreamCrashResume terminates the Telegram adapter
// mid-stream, restarts it, and verifies it resumes from the saved checkpoint with 0 duplicate messages.
func TestTelegramOffsetPersistence_MidStreamCrashResume(t *testing.T) {
	fake := newFakeTelegram(t)
	token := "301:crash-stream"
	fake.addBot(token, 301, "stream_bot")

	// 1. Initial batch of 5 messages (UpdateID 1..5)
	updatesBatch1 := make([]botapi.Update, 0, 5)
	for i := 1; i <= 5; i++ {
		updatesBatch1 = append(updatesBatch1, botapi.Update{
			UpdateID: int64(i),
			Message: &botapi.Message{
				MessageID: int64(i),
				Date:      100 + int64(i),
				Chat:      botapi.Chat{ID: 100, Type: "private"},
				From:      &botapi.User{ID: 100, FirstName: "User"},
				Text:      fmt.Sprintf("Message %d", i),
			},
		})
	}
	fake.mu.Lock()
	fake.updates[token] = updatesBatch1
	fake.mu.Unlock()

	databasePath := filepath.Join(t.TempDir(), "telegram.db")
	firstPublisher := &recordingPublisher{}
	manager1 := newTestManager(t, databasePath, fake, firstPublisher)

	if _, err := manager1.Create(t.Context(), "channel-stream", token); err != nil {
		t.Fatal(err)
	}

	// Wait for first 5 messages to be processed
	waitFor(t, func() bool { return firstPublisher.count(messaging.EventMessageCreated) == 5 })

	// Terminate the adapter mid-stream abruptly
	if err := manager1.Close(); err != nil {
		t.Fatal(err)
	}

	// 2. Add next batch of 5 messages (UpdateID 6..10) while adapter is down
	fake.mu.Lock()
	for i := 6; i <= 10; i++ {
		fake.updates[token] = append(fake.updates[token], botapi.Update{
			UpdateID: int64(i),
			Message: &botapi.Message{
				MessageID: int64(i),
				Date:      100 + int64(i),
				Chat:      botapi.Chat{ID: 100, Type: "private"},
				From:      &botapi.User{ID: 100, FirstName: "User"},
				Text:      fmt.Sprintf("Message %d", i),
			},
		})
	}
	fake.mu.Unlock()

	// 3. Restart adapter pointing to the same persistent SQLite database
	secondPublisher := &recordingPublisher{}
	manager2 := newTestManager(t, databasePath, fake, secondPublisher)

	// Wait for manager2 to resume and process remaining messages
	waitFor(t, func() bool { return secondPublisher.count(messaging.EventMessageCreated) == 5 })

	snap, err := manager2.Snapshot("channel-stream")
	assert.NoError(t, err)
	assert.Equal(t, "channel-stream", snap.ChannelID)

	// Verify manager2 only requested updates starting from offset 6
	waitFor(t, func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		return containsOffset(fake.offsets[token], 6) && containsOffset(fake.offsets[token], 11)
	})

	// Assert exactly 0 duplicates across the stream: first publisher got 1..5, second got 6..10
	firstPublisher.mu.Lock()
	ev1 := firstPublisher.events
	firstPublisher.mu.Unlock()

	secondPublisher.mu.Lock()
	ev2 := secondPublisher.events
	secondPublisher.mu.Unlock()

	assert.Equal(t, 5, firstPublisher.count(messaging.EventMessageCreated), "First manager processed exactly 5 messages")
	assert.Equal(t, 5, secondPublisher.count(messaging.EventMessageCreated), "Second manager processed exactly 5 new messages")

	allSeenIDs := make(map[string]bool)
	for _, e := range ev1 {
		if e.Message != nil {
			assert.False(t, allSeenIDs[e.Message.ProviderMessageID], "duplicate message in batch 1")
			allSeenIDs[e.Message.ProviderMessageID] = true
		}
	}
	for _, e := range ev2 {
		if e.Message != nil {
			assert.False(t, allSeenIDs[e.Message.ProviderMessageID], "duplicate message in batch 2 (replay leak)")
			allSeenIDs[e.Message.ProviderMessageID] = true
		}
	}
	assert.Equal(t, 10, len(allSeenIDs), "Exactly 10 unique messages processed with zero duplicates")
}

func TestRetryCanReplaceARevokedBotToken(t *testing.T) {
	fake := newFakeTelegram(t)
	fake.addBot("251:old", 251, "old_bot")
	fake.addBot("252:new-secret", 252, "new_bot")
	manager := newTestManager(t, filepath.Join(t.TempDir(), "telegram.db"), fake, &recordingPublisher{})
	if _, err := manager.Create(t.Context(), "channel-retry", "251:old"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := manager.Retry(t.Context(), "channel-retry", "252:new-secret")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.RemoteAccountID != "@new_bot" {
		t.Fatalf("remote account = %q", snapshot.RemoteAccountID)
	}
	var encrypted string
	if err := manager.db.QueryRow(`SELECT encrypted_token FROM telegram_sessions WHERE channel_id = 'channel-retry'`).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(encrypted, "new-secret") || strings.Contains(encrypted, "251:old") {
		t.Fatal("replacement bot token was not encrypted")
	}
}

func containsOffset(values []int64, want int64) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestManagerRecordsPermanentOutboundFailureOnce(t *testing.T) {
	fake := newFakeTelegram(t)
	const token = "301:blocked"
	fake.addBot(token, 301, "blocked_bot")
	fake.failSend[token] = true
	publisher := &recordingPublisher{}
	manager := newTestManager(t, filepath.Join(t.TempDir(), "telegram.db"), fake, publisher)
	if _, err := manager.Create(t.Context(), "channel-failure", token); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		snapshot, _ := manager.Snapshot("channel-failure")
		return snapshot.State == messaging.ConnectionConnected
	})
	command := messaging.Command{SchemaVersion: messaging.SchemaVersion, ID: "command-1", Kind: messaging.CommandSendMessage, Provider: messaging.ProviderTelegram, ChannelID: "channel-failure", CreatedAt: time.Now().UTC(), MessageID: "local-message", Message: &messaging.Message{ExternalThreadID: "42", Direction: messaging.DirectionOutbound, Sender: messaging.Sender{ExternalID: "business"}, ContentType: messaging.ContentText, Text: "hello", ProviderTimestamp: time.Now().UTC()}}
	if err := manager.Send(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if err := manager.Send(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return publisher.count(messaging.EventReceiptChanged) == 1 })
	fake.mu.Lock()
	calls := fake.sendCalls[token]
	fake.mu.Unlock()
	if calls != 1 {
		t.Fatalf("send calls = %d, want 1", calls)
	}
}

func TestManagerExecutesSupportedOutboundCommands(t *testing.T) {
	fake := newFakeTelegram(t)
	const token = "351:commands"
	fake.addBot(token, 351, "commands_bot")
	publisher := &recordingPublisher{}
	manager := newTestManager(t, filepath.Join(t.TempDir(), "telegram.db"), fake, publisher)
	manager.SetMediaSource(staticMediaSource{})
	if _, err := manager.Create(t.Context(), "channel-commands", token); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		snapshot, _ := manager.Snapshot("channel-commands")
		return snapshot.State == messaging.ConnectionConnected
	})
	now := time.Now().UTC()
	commands := []messaging.Command{
		{SchemaVersion: 1, ID: "send-media", Kind: messaging.CommandSendMessage, Provider: messaging.ProviderTelegram, ChannelID: "channel-commands", CreatedAt: now, MessageID: "local-media", Message: &messaging.Message{ExternalThreadID: "42", Direction: messaging.DirectionOutbound, Sender: messaging.Sender{ExternalID: "business"}, ContentType: messaging.ContentDocument, Text: "caption", Media: &messaging.Media{ID: "media-1", Filename: "guide.pdf", MIMEType: "application/pdf", SizeBytes: 8}, ProviderTimestamp: now}},
		{SchemaVersion: 1, ID: "edit-text", Kind: messaging.CommandEditMessage, Provider: messaging.ProviderTelegram, ChannelID: "channel-commands", CreatedAt: now, MessageID: "local-edit", Message: &messaging.Message{ProviderMessageID: "50", ExternalThreadID: "42", Direction: messaging.DirectionOutbound, ContentType: messaging.ContentText, Text: "edited", ProviderTimestamp: now}},
		{SchemaVersion: 1, ID: "delete-message", Kind: messaging.CommandDeleteMessage, Provider: messaging.ProviderTelegram, ChannelID: "channel-commands", CreatedAt: now, MessageID: "local-delete", Message: &messaging.Message{ProviderMessageID: "51", ExternalThreadID: "42", ProviderTimestamp: now}},
		{SchemaVersion: 1, ID: "react-message", Kind: messaging.CommandChangeReaction, Provider: messaging.ProviderTelegram, ChannelID: "channel-commands", CreatedAt: now, MessageID: "local-react", Reaction: &messaging.Reaction{ProviderMessageID: "52", SenderExternalID: "42", Emoji: "👍", ProviderTimestamp: now}},
	}
	for _, command := range commands {
		if err := manager.Send(t.Context(), command); err != nil {
			t.Fatalf("Send(%s): %v", command.Kind, err)
		}
	}
	waitFor(t, func() bool {
		return publisher.count(messaging.EventMessageCreated) == 1 && publisher.count(messaging.EventMessageEdited) == 1 && publisher.count(messaging.EventMessageDeleted) == 1 && publisher.count(messaging.EventReactionChanged) == 1
	})
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, method := range []string{"sendDocument", "editMessageText", "deleteMessage", "setMessageReaction"} {
		if fake.methods[token+":"+method] != 1 {
			t.Errorf("%s calls = %d, want 1", method, fake.methods[token+":"+method])
		}
	}
}

func TestManagerRejectsDuplicateBotAcrossChannels(t *testing.T) {
	fake := newFakeTelegram(t)
	const token = "401:same"
	fake.addBot(token, 401, "same_bot")
	manager := newTestManager(t, filepath.Join(t.TempDir(), "telegram.db"), fake, &recordingPublisher{})
	if _, err := manager.Create(t.Context(), "channel-one", token); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Create(t.Context(), "channel-two", token); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("error = %v, want ErrAlreadyExists", err)
	}
}

func TestPrivateChatID(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"0", "-100", "group", ""} {
		t.Run(fmt.Sprintf("invalid_%s", strconv.Quote(value)), func(t *testing.T) {
			if _, err := privateChatID(value); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestManagerConcurrentSends(t *testing.T) {
	fake := newFakeTelegram(t)
	const token = "501:concurrent"
	fake.addBot(token, 501, "concurrent_bot")
	publisher := &recordingPublisher{}
	manager := newTestManager(t, filepath.Join(t.TempDir(), "telegram.db"), fake, publisher)

	if _, err := manager.Create(t.Context(), "channel-concurrent", token); err != nil {
		t.Fatal(err)
	}

	waitFor(t, func() bool {
		snapshot, _ := manager.Snapshot("channel-concurrent")
		return snapshot.State == messaging.ConnectionConnected
	})

	const count = 10
	var wg sync.WaitGroup
	errCh := make(chan error, count)
	now := time.Now().UTC()

	for i := 0; i < count; i++ {
		wg.Add(1)
		msgID := fmt.Sprintf("msg-%d", i)
		cmd := messaging.Command{
			SchemaVersion: 1,
			ID:            "cmd-" + msgID,
			Kind:          messaging.CommandSendMessage,
			Provider:      messaging.ProviderTelegram,
			ChannelID:     "channel-concurrent",
			CreatedAt:     now,
			MessageID:     msgID,
			Message: &messaging.Message{
				ExternalThreadID:  "42",
				Direction:         messaging.DirectionOutbound,
				Sender:            messaging.Sender{ExternalID: "business"},
				ContentType:       messaging.ContentText,
				Text:              "Concurrent hello " + msgID,
				ProviderTimestamp: now,
			},
		}
		go func(c messaging.Command) {
			defer wg.Done()
			errCh <- manager.Send(t.Context(), c)
		}(cmd)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Errorf("Send failed: %v", err)
		}
	}

	waitFor(t, func() bool {
		return publisher.count(messaging.EventMessageCreated) == count
	})
}
