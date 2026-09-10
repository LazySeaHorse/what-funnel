package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func cleanupIntegrationTestData() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return
	}
	defer pool.Close()

	// Never touch foo@barr.com or account Foobarr
	_, _ = pool.Exec(ctx, `
		DELETE FROM accounts
		WHERE id NOT IN (
			SELECT account_id FROM users WHERE email = 'foo@barr.com'
		) AND (
			id IN (
				SELECT DISTINCT account_id FROM users
				WHERE email LIKE '%@example.com' OR email LIKE '%@e2e.local' OR email LIKE '%@local.test'
			)
			OR name LIKE 'E2E %'
			OR name LIKE 'TestTenant%'
		);

		DELETE FROM users
		WHERE (email LIKE '%@example.com' OR email LIKE '%@e2e.local' OR email LIKE '%@local.test')
		  AND email != 'foo@barr.com';
	`)
}

func TestMain(m *testing.M) {
	cleanupIntegrationTestData()
	code := m.Run()
	cleanupIntegrationTestData()
	os.Exit(code)
}
