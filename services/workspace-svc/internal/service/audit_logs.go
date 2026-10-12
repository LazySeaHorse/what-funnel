package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db/dbgen"
)

const (
	DefaultAuditLogLimit = 50
	MaxAuditLogLimit     = 200
)

// ErrInvalidAuditCursor is returned for a malformed pagination cursor.
var ErrInvalidAuditCursor = errors.New("invalid cursor")

// AuditLogFilter selects audit log rows. Zero-valued fields are not applied.
type AuditLogFilter struct {
	Limit       int
	Cursor      string
	ActorUserID *uuid.UUID
	Action      string
	From, To    *time.Time
}

// AuditLogItem is one audit log entry as returned by the API.
type AuditLogItem struct {
	ID          uuid.UUID       `json:"id"`
	ActorUserID *uuid.UUID      `json:"actor_user_id"`
	ActorName   *string         `json:"actor_name"`
	ActorEmail  *string         `json:"actor_email"`
	Action      string          `json:"action"`
	TargetType  string          `json:"target_type"`
	TargetID    *uuid.UUID      `json:"target_id"`
	Metadata    json.RawMessage `json:"metadata"`
	CreatedAt   time.Time       `json:"created_at"`
}

// AuditLogPage is a page of audit log entries.
type AuditLogPage struct {
	Items      []AuditLogItem `json:"items"`
	NextCursor *string        `json:"next_cursor"`
}

// EncodeAuditCursor builds the opaque cursor for the given row position.
func EncodeAuditCursor(createdAt time.Time, id uuid.UUID) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeAuditCursor(c string) (time.Time, uuid.UUID, error) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidAuditCursor
	}
	ts, idStr, ok := strings.Cut(string(b), "|")
	if !ok {
		return time.Time{}, uuid.Nil, ErrInvalidAuditCursor
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidAuditCursor
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidAuditCursor
	}
	return t, id, nil
}

// ListAuditLogs returns the account's audit log, newest first. Always scoped to accountID.
func (svc *Service) ListAuditLogs(ctx context.Context, accountID uuid.UUID, f AuditLogFilter) (*AuditLogPage, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = DefaultAuditLogLimit
	}
	if limit > MaxAuditLogLimit {
		limit = MaxAuditLogLimit
	}
	params := dbgen.ListAuditLogsParams{
		AccountID:   accountID,
		ActorUserID: f.ActorUserID,
		RowLimit:    int32(limit + 1),
	}
	if f.Action != "" {
		a := f.Action
		params.Action = &a
	}
	if f.From != nil {
		params.FromTs = pgtype.Timestamptz{Time: *f.From, Valid: true}
	}
	if f.To != nil {
		params.ToTs = pgtype.Timestamptz{Time: *f.To, Valid: true}
	}
	if f.Cursor != "" {
		ct, cid, err := decodeAuditCursor(f.Cursor)
		if err != nil {
			return nil, err
		}
		params.CursorCreatedAt = pgtype.Timestamptz{Time: ct, Valid: true}
		params.CursorID = &cid
	}

	rows, err := dbgen.New(svc.pool).ListAuditLogs(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list audit logs: %w", err)
	}
	page := &AuditLogPage{Items: make([]AuditLogItem, 0, len(rows))}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		c := EncodeAuditCursor(last.CreatedAt, last.ID)
		page.NextCursor = &c
	}
	for _, r := range rows {
		meta := r.Metadata
		if len(meta) == 0 {
			meta = json.RawMessage("{}")
		}
		page.Items = append(page.Items, AuditLogItem{
			ID: r.ID, ActorUserID: r.ActorUserID, ActorName: r.ActorName, ActorEmail: r.ActorEmail,
			Action: r.Action, TargetType: r.TargetType, TargetID: r.TargetID,
			Metadata: meta, CreatedAt: r.CreatedAt,
		})
	}
	return page, nil
}
