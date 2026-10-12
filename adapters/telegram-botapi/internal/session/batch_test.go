package session

import (
	"path/filepath"
	"testing"

	"github.com/whatfunnel/whatfunnel/adapters/telegram-botapi/internal/botapi"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

func TestManagerStoresAMixedUpdateBatchInOneTransaction(t *testing.T) {
	fake := newFakeTelegram(t)
	const token = "404:batch"
	fake.addBot(token, 404, "batch_bot")
	publisher := &recordingPublisher{}
	manager := newTestManager(t, filepath.Join(t.TempDir(), "telegram.db"), fake, publisher)
	if _, err := manager.Create(t.Context(), "channel-batch", token); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		snapshot, _ := manager.Snapshot("channel-batch")
		return snapshot.State == messaging.ConnectionConnected
	})

	// Count every write to the offset column; one batch must produce one write.
	if _, err := manager.db.ExecContext(t.Context(), `
		CREATE TABLE offset_writes (id INTEGER PRIMARY KEY);
		CREATE TRIGGER count_offset_writes AFTER UPDATE OF update_offset ON telegram_sessions
		BEGIN INSERT INTO offset_writes (id) VALUES (NULL); END;
	`); err != nil {
		t.Fatal(err)
	}
	private := func(id int64) *botapi.Message {
		return &botapi.Message{MessageID: id, Date: 100, Chat: botapi.Chat{ID: 41, Type: "private"}, From: &botapi.User{ID: 41, FirstName: "U"}, Text: "hi"}
	}
	fake.mu.Lock()
	fake.updates[token] = []botapi.Update{
		{UpdateID: 1, Message: private(1)},
		{UpdateID: 2}, // unsupported update: no event, but the offset must still advance past it
		{UpdateID: 3, Message: private(3)},
		{UpdateID: 4}, // trailing unsupported update
	}
	fake.mu.Unlock()

	waitFor(t, func() bool { return publisher.count(messaging.EventMessageCreated) == 2 })
	var offset, writes int64
	if err := manager.db.QueryRow(`SELECT update_offset FROM telegram_sessions WHERE channel_id = ?`, "channel-batch").Scan(&offset); err != nil {
		t.Fatal(err)
	}
	if err := manager.db.QueryRow(`SELECT COUNT(*) FROM offset_writes`).Scan(&writes); err != nil {
		t.Fatal(err)
	}
	if offset != 5 {
		t.Fatalf("offset = %d, want 5", offset)
	}
	if writes != 1 {
		t.Fatalf("offset writes = %d, want 1 per batch", writes)
	}
}
