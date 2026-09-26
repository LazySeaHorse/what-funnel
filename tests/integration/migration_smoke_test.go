package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestMigrationSmoke_BlankDatabase(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping migration smoke test in short mode")
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	adminConn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Skipf("cannot connect to postgres admin database: %v", err)
	}
	defer adminConn.Close(ctx)

	// Generate clean temp database name
	tempDBName := fmt.Sprintf("wf_smoke_%s", strings.ReplaceAll(uuid.NewString()[:8], "-", ""))

	// Create blank database
	_, err = adminConn.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s;", tempDBName))
	require.NoError(t, err, "failed to create blank test database")

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		// Terminate connections and drop
		_, _ = adminConn.Exec(cleanupCtx, fmt.Sprintf(`
			SELECT pg_terminate_backend(pid)
			FROM pg_stat_activity
			WHERE datname = '%s' AND pid <> pg_backend_pid();
		`, tempDBName))
		_, _ = adminConn.Exec(cleanupCtx, fmt.Sprintf("DROP DATABASE IF EXISTS %s;", tempDBName))
	})

	root := findRepoRoot(t)
	migrationsDir := filepath.Join(root, "packages", "go-common", "migrations")

	// Construct DSN for the new blank db
	baseParts := strings.Split(dsn, "/")
	prefix := strings.Join(baseParts[:len(baseParts)-1], "/")
	tempDSN := fmt.Sprintf("%s/%s?sslmode=disable", prefix, tempDBName)

	// Run goose up
	gooseCmd := exec.Command("goose", "-dir", migrationsDir, "postgres", tempDSN, "up")
	output, err := gooseCmd.CombinedOutput()
	require.NoError(t, err, "goose up failed on blank database:\n%s", string(output))

	// Verify status reports 0 pending migrations
	statusCmd := exec.Command("goose", "-dir", migrationsDir, "postgres", tempDSN, "status")
	statusOutput, err := statusCmd.CombinedOutput()
	require.NoError(t, err, "goose status failed:\n%s", string(statusOutput))

	statusStr := string(statusOutput)
	require.NotContains(t, statusStr, "Pending", "Expected 0 pending migrations after goose up")
	require.Contains(t, statusStr, "00001_foundation_schema.sql")
	require.Contains(t, statusStr, "00021_performance_indexes.sql")
}
