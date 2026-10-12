// Package audit provides a small helper for writing audit_logs rows.
// Services call it explicitly at every point of mutation — no magic reflection.
// See spec §7 and build prompt §7.
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Execer is what a Writer writes through: *pgxpool.Pool, *pgxpool.Conn and
// pgx.Tx all satisfy it.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Entry represents all inputs needed to write one audit log row.
type Entry struct {
	AccountID   uuid.UUID
	ActorUserID *uuid.UUID // nil for system-generated events
	Action      string
	TargetType  string
	TargetID    *uuid.UUID // nil when target has no UUID (e.g. login events)
	Metadata    map[string]any
}

// Writer writes audit log entries to Postgres.
type Writer struct {
	db Execer
}

// NewWriter creates a Writer backed by the given pool, connection or transaction.
func NewWriter(db Execer) *Writer {
	return &Writer{db: db}
}

// NewWriterFromTx creates a Writer backed by a transaction.
// Use this to keep the audit write atomic with the mutation.
func NewWriterFromTx(tx pgx.Tx) *Writer {
	return NewWriter(tx)
}

// Write persists an audit log entry and returns any marshal or database error.
// For transactional guarantees (audit row or nothing), use NewWriterFromTx
// and manage the transaction in the caller.
func (w *Writer) Write(ctx context.Context, e Entry) error {
	meta, err := json.Marshal(e.Metadata)
	if err != nil {
		return fmt.Errorf("audit: marshal metadata: %w", err)
	}
	if meta == nil {
		meta = []byte("{}")
	}

	_, err = w.db.Exec(ctx, `
		INSERT INTO audit_logs
			(account_id, actor_user_id, action, target_type, target_id, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		e.AccountID,
		e.ActorUserID,
		e.Action,
		e.TargetType,
		e.TargetID,
		meta,
	)
	if err != nil {
		return fmt.Errorf("audit: write log: %w", err)
	}
	return nil
}

// TxBeginner starts transactions; *pgxpool.Pool satisfies it.
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// RunInTx runs fn inside a transaction, writes the audit entry fn returns
// through the same transaction, and commits. Any error — from fn, the audit
// write or the commit — rolls the whole transaction back, so the mutation and
// its audit row are atomic. fn returns the entry because some metadata is only
// known once the mutation has run.
func RunInTx(ctx context.Context, db TxBeginner, fn func(tx pgx.Tx) (Entry, error)) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	entry, err := fn(tx)
	if err != nil {
		return err
	}
	if err := NewWriterFromTx(tx).Write(ctx, entry); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------------
// Action constants — keep these centralised so they're consistent across
// services and easy to grep.
// ---------------------------------------------------------------------------

const (
	ActionAccountCreated     = "account.created"
	ActionUserCreated        = "user.created"
	ActionUserCreatedByAdmin = "user.created_by_admin"
	ActionUserDeleted        = "user.deleted"
	ActionUserPasswordReset  = "user.password_reset"
	ActionUserRoleChanged    = "user.role_changed"
	ActionChannelDeleted     = "channel.deleted"
	ActionLogin              = "user.login"
	ActionLogout             = "user.logout"
	ActionPipelineCreated    = "pipeline.created"
	ActionPipelineUpdated    = "pipeline.updated"
)

// Target type constants.
const (
	TargetAccount  = "account"
	TargetChannel  = "channel"
	TargetUser     = "user"
	TargetPipeline = "lead_pipeline"
)
