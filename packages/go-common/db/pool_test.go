package db_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db"
)

// testPool returns a pgxpool connected to the test database, or skips the test
// if DATABASE_URL is not set. Integration tests require `make up` first.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("database unavailable in CI: %v", err)
		}
		t.Skipf("skipping integration test: cannot connect to postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
