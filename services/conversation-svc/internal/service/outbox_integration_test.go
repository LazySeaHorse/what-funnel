package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

type outboxFixture struct {
	pool           *pgxpool.Pool
	accountID      uuid.UUID
	channelID      uuid.UUID
	conversationID uuid.UUID
}

func newOutboxFixture(t *testing.T) outboxFixture {
	t.Helper()
	pool := testDBPool(t)
	ctx := context.Background()
	f := outboxFixture{pool: pool}
	mustExec := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	mustExec(pool.QueryRow(ctx, `INSERT INTO accounts (name) VALUES ('outbox-fixture') RETURNING id`).Scan(&f.accountID))
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM message_outbox WHERE account_id = $1`, f.accountID)
		_, _ = pool.Exec(bg, `DELETE FROM messages WHERE account_id = $1`, f.accountID)
		_, _ = pool.Exec(bg, `DELETE FROM conversations WHERE account_id = $1`, f.accountID)
		_, _ = pool.Exec(bg, `DELETE FROM contacts WHERE account_id = $1`, f.accountID)
		_, _ = pool.Exec(bg, `DELETE FROM channels WHERE account_id = $1`, f.accountID)
		_, _ = pool.Exec(bg, `DELETE FROM accounts WHERE id = $1`, f.accountID)
	})
	mustExec(pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, provider, label, status, capabilities)
		VALUES ($1, 'telegram', 'telegram', 'outbox bot', 'connected', '{}') RETURNING id
	`, f.accountID).Scan(&f.channelID))
	var contactID uuid.UUID
	mustExec(pool.QueryRow(ctx, `
		INSERT INTO contacts (account_id, channel_id, external_identity, display_name)
		VALUES ($1, $2, 'outbox-thread', 'x') RETURNING id
	`, f.accountID, f.channelID).Scan(&contactID))
	mustExec(pool.QueryRow(ctx, `
		INSERT INTO conversations (account_id, contact_id, channel_id, external_thread_id)
		VALUES ($1, $2, $3, 'outbox-thread') RETURNING id
	`, f.accountID, contactID, f.channelID).Scan(&f.conversationID))
	return f
}

// queueSend inserts a queued outbound message and its outbox command. The
// created_at offsets keep the FIFO order deterministic.
func (f outboxFixture) queueSend(t *testing.T, text string, age time.Duration) (messageID uuid.UUID, commandID string) {
	t.Helper()
	ctx := context.Background()
	created := time.Now().UTC().Add(-age)
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO messages (account_id, conversation_id, direction, sender_type, content_type, content, delivery_status, created_at)
		VALUES ($1, $2, 'outbound', 'human', 'text', jsonb_build_object('text', $3::TEXT), 'queued', $4) RETURNING id
	`, f.accountID, f.conversationID, text, created).Scan(&messageID); err != nil {
		t.Fatal(err)
	}
	commandID = "send:" + messageID.String()
	command := messaging.Command{
		SchemaVersion: messaging.SchemaVersion,
		ID:            commandID,
		Kind:          messaging.CommandSendMessage,
		Provider:      messaging.ProviderTelegram,
		ChannelID:     f.channelID.String(),
		CreatedAt:     created,
		MessageID:     messageID.String(),
		Message: &messaging.Message{
			ExternalThreadID: "outbox-thread", Direction: messaging.DirectionOutbound,
			Sender:      messaging.Sender{ExternalID: "business"},
			ContentType: messaging.ContentText, Text: text, ProviderTimestamp: created,
		},
	}
	if err := command.Validate(); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `
		INSERT INTO message_outbox (account_id, channel_id, message_id, provider, command, created_at)
		VALUES ($1, $2, $3, 'telegram', $4, $5)
	`, f.accountID, f.channelID, messageID, payload, created); err != nil {
		t.Fatal(err)
	}
	return messageID, commandID
}

func TestClaimOutboxCommand_PreservesConversationOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	f := newOutboxFixture(t)
	ctx := context.Background()
	svc := NewOutboxService(f.pool, nil)

	firstID, firstCommand := f.queueSend(t, "first", 2*time.Minute)
	_, secondCommand := f.queueSend(t, "second", time.Minute)

	// The earlier command is backing off after a failed attempt.
	if _, err := f.pool.Exec(ctx, `
		UPDATE message_outbox SET attempts = 1, available_at = NOW() + INTERVAL '1 hour'
		WHERE message_id = $1
	`, firstID); err != nil {
		t.Fatal(err)
	}

	// Neither the background worker nor the immediate nudge may let the later
	// message overtake the backing-off one.
	for _, commandID := range []string{"", secondCommand} {
		claimed, err := svc.claimOutboxCommand(ctx, uuid.NewString(), commandID)
		if err != nil {
			t.Fatal(err)
		}
		if claimed != nil {
			t.Fatalf("claimed %s while an earlier command for the conversation is undelivered", claimed.command.ID)
		}
	}

	// Once the earlier command is ready it goes first, and the nudge only
	// claims the command it was asked for.
	if _, err := f.pool.Exec(ctx, `UPDATE message_outbox SET available_at = NOW() WHERE message_id = $1`, firstID); err != nil {
		t.Fatal(err)
	}
	if claimed, err := svc.claimOutboxCommand(ctx, uuid.NewString(), secondCommand); err != nil || claimed != nil {
		t.Fatalf("specific claim of the later command = %v, %v; want none while the first is undelivered", claimed, err)
	}
	claimed, err := svc.claimOutboxCommand(ctx, uuid.NewString(), firstCommand)
	if err != nil || claimed == nil || claimed.command.ID != firstCommand {
		t.Fatalf("specific claim = %v, %v; want %s", claimed, err, firstCommand)
	}
}

func TestReleaseOutboxCommand_AbandonsAfterMaxAttempts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	f := newOutboxFixture(t)
	ctx := context.Background()
	svc := NewOutboxService(f.pool, nil)

	messageID, commandID := f.queueSend(t, "doomed", time.Minute)
	_, laterCommand := f.queueSend(t, "later", 30*time.Second)
	dispatchErr := errors.New("redis unavailable")

	// Every failed attempt below the cap just reschedules the command.
	for attempt := 1; attempt < maxOutboxAttempts; attempt++ {
		if _, err := f.pool.Exec(ctx, `UPDATE message_outbox SET available_at = NOW() WHERE message_id = $1`, messageID); err != nil {
			t.Fatal(err)
		}
		claimID := uuid.NewString()
		claimed, err := svc.claimOutboxCommand(ctx, claimID, commandID)
		if err != nil || claimed == nil {
			t.Fatalf("attempt %d claim = %v, %v", attempt, claimed, err)
		}
		if err := svc.releaseOutboxCommand(ctx, claimed, claimID, dispatchErr); err != nil {
			t.Fatal(err)
		}
		var status string
		if err := f.pool.QueryRow(ctx, `SELECT delivery_status FROM messages WHERE id = $1`, messageID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "queued" {
			t.Fatalf("attempt %d: delivery_status = %q, want queued", attempt, status)
		}
	}

	// The final attempt abandons the command and fails the message.
	if _, err := f.pool.Exec(ctx, `UPDATE message_outbox SET available_at = NOW() WHERE message_id = $1`, messageID); err != nil {
		t.Fatal(err)
	}
	claimID := uuid.NewString()
	claimed, err := svc.claimOutboxCommand(ctx, claimID, commandID)
	if err != nil || claimed == nil {
		t.Fatalf("final claim = %v, %v", claimed, err)
	}
	if err := svc.releaseOutboxCommand(ctx, claimed, claimID, dispatchErr); err != nil {
		t.Fatal(err)
	}
	var status string
	var dispatched bool
	if err := f.pool.QueryRow(ctx, `
		SELECT m.delivery_status, o.dispatched_at IS NOT NULL
		FROM messages m JOIN message_outbox o ON o.message_id = m.id WHERE m.id = $1
	`, messageID).Scan(&status, &dispatched); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || !dispatched {
		t.Fatalf("after max attempts: delivery_status = %q, outbox closed = %v; want failed, true", status, dispatched)
	}

	// The abandoned command no longer blocks the conversation.
	next, err := svc.claimOutboxCommand(ctx, uuid.NewString(), laterCommand)
	if err != nil || next == nil {
		t.Fatalf("later command claim = %v, %v; want it deliverable", next, err)
	}
}
