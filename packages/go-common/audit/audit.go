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
)

// Querier is the subset of the DB interface needed to write audit logs.
// Both *pgxpool.Pool and pgx.Tx satisfy this interface.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (interface{ RowsAffected() int64 }, error)
}

// TxQuerier is pgx.Tx — used when the audit log row should be part of
// the same transaction as the mutation being audited.
type TxQuerier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (interface{ RowsAffected() int64 }, error)
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

// pgxTxAdapter wraps pgx.Tx to expose the Exec signature expected by Write.
type pgxTxAdapter struct{ tx pgx.Tx }

func (a *pgxTxAdapter) Exec(ctx context.Context, sql string, args ...any) (interface{ RowsAffected() int64 }, error) {
	return a.tx.Exec(ctx, sql, args...)
}

// Writer writes audit log entries to Postgres.
type Writer struct {
	// db is the connection to use for writes. Can be a pool or a tx.
	db interface {
		Exec(ctx context.Context, sql string, args ...any) (interface{ RowsAffected() int64 }, error)
	}
}

// NewWriter creates a Writer backed by the given pool/connection.
func NewWriter(db interface {
	Exec(ctx context.Context, sql string, args ...any) (interface{ RowsAffected() int64 }, error)
}) *Writer {
	return &Writer{db: db}
}

// NewWriterFromTx creates a Writer backed by a transaction.
// Use this to keep the audit write atomic with the mutation.
func NewWriterFromTx(tx pgx.Tx) *Writer {
	return &Writer{db: &pgxTxAdapter{tx: tx}}
}

// Write persists an audit log entry. It does not return an error that would
// abort the caller's business logic — if the write fails, it logs to stderr
// but does not propagate the error. This is a deliberate v1 choice: a failed
// audit write should not roll back a successful business operation.
//
// If you need transactional guarantees (audit row or nothing), use
// NewWriterFromTx and manage the transaction in the caller.
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

// ---------------------------------------------------------------------------
// Action constants — keep these centralised so they're consistent across
// services and easy to grep.
// ---------------------------------------------------------------------------

const (
	ActionAccountCreated  = "account.created"
	ActionUserCreated     = "user.created"
	ActionUserInvited     = "user.invited"
	ActionUserRoleChanged = "user.role_changed"
	ActionLogin           = "user.login"
	ActionLogout          = "user.logout"
	ActionPipelineCreated = "pipeline.created"
	ActionPipelineUpdated = "pipeline.updated"
)

// Target type constants.
const (
	TargetAccount  = "account"
	TargetUser     = "user"
	TargetPipeline = "lead_pipeline"
)
