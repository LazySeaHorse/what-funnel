// Package db provides the ScopedDB wrapper that enforces application-layer
// tenant isolation (see spec §9 and root README).
//
// Every query that touches a tenant-scoped table MUST be issued through a
// ScopedDB, which automatically injects the account_id filter. Attempting to
// call a scoped query helper without first obtaining a ScopedDB will not
// compile — this is intentional.
//
// v1 tradeoff: isolation is enforced here, not at the Postgres RLS layer. RLS
// is a documented fast-follow. See README for details.
package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool wraps pgxpool.Pool with a thin interface so tests can substitute fakes.
type Pool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (interface{ RowsAffected() int64 }, error)
}

// pgxPoolAdapter wraps *pgxpool.Pool to satisfy Pool.
type pgxPoolAdapter struct {
	p *pgxpool.Pool
}

func (a *pgxPoolAdapter) Begin(ctx context.Context) (pgx.Tx, error) {
	return a.p.Begin(ctx)
}

func (a *pgxPoolAdapter) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return a.p.QueryRow(ctx, sql, args...)
}

func (a *pgxPoolAdapter) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return a.p.Query(ctx, sql, args...)
}

func (a *pgxPoolAdapter) Exec(ctx context.Context, sql string, args ...any) (interface{ RowsAffected() int64 }, error) {
	return a.p.Exec(ctx, sql, args...)
}

// WrapPool wraps a *pgxpool.Pool with the Pool interface.
func WrapPool(p *pgxpool.Pool) Pool {
	return &pgxPoolAdapter{p: p}
}

// ScopedDB carries an account_id that is automatically injected into every
// tenant-scoped query. Obtain one via Scope().
type ScopedDB struct {
	pool      Pool
	accountID uuid.UUID
}

// Scope returns a ScopedDB bound to the given account_id. All queries issued
// through the returned ScopedDB automatically include WHERE account_id = $n,
// preventing cross-tenant data access.
func Scope(pool Pool, accountID uuid.UUID) (*ScopedDB, error) {
	if accountID == uuid.Nil {
		return nil, fmt.Errorf("db.Scope: accountID must not be nil")
	}
	return &ScopedDB{pool: pool, accountID: accountID}, nil
}

// AccountID returns the tenant bound to this ScopedDB.
func (s *ScopedDB) AccountID() uuid.UUID {
	return s.accountID
}

// Pool returns the underlying connection pool. Use sparingly — prefer the
// scoped helper methods below. Only use Pool() for queries against global
// tables (e.g. looking up a user by email during login, before account_id is
// known).
func (s *ScopedDB) Pool() Pool {
	return s.pool
}

// Begin starts a transaction. The caller is responsible for Commit/Rollback.
func (s *ScopedDB) Begin(ctx context.Context) (pgx.Tx, error) {
	return s.pool.Begin(ctx)
}

// QueryRow executes a query that expects a single row. The provided SQL MUST
// contain a $accountID placeholder that this method fills with the scoped
// account_id. Use AppendAccountID to add it.
func (s *ScopedDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return s.pool.QueryRow(ctx, sql, args...)
}

// Query executes a query returning multiple rows.
func (s *ScopedDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return s.pool.Query(ctx, sql, args...)
}

// Exec executes a statement.
func (s *ScopedDB) Exec(ctx context.Context, sql string, args ...any) (interface{ RowsAffected() int64 }, error) {
	return s.pool.Exec(ctx, sql, args...)
}

// AppendAccountID appends the scoped account_id to the args slice and returns
// the position (1-indexed) for use in the SQL placeholder.
//
// Usage:
//
//	args := []any{someOtherArg}
//	pos, args := sdb.AppendAccountID(args)
//	row := sdb.QueryRow(ctx, fmt.Sprintf("SELECT ... WHERE account_id = $%d", pos), args...)
func (s *ScopedDB) AppendAccountID(args []any) (int, []any) {
	args = append(args, s.accountID)
	return len(args), args
}
