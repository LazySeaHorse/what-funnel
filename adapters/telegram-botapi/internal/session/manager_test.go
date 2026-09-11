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
	failSend  map[string]bool
}

func newFakeTelegram(t *testing.T) *fakeTelegram {
	t.Helper()
	fake := &fakeTelegram{bots: make(map[string]botapi.User), updates: make(map[string][]botapi.Update), offsets: make(map[string][]int64), sendCalls: make(map[string]int), failSend: make(map[string]bool)}
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
	default:
		f.respond(w, true)
	}
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
