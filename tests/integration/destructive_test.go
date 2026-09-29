package integration

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
)

// destructiveEnv opts in to tests that stop, pause, restart or kill containers of
// the shared dev stack. They disrupt every other user of that stack (other test
// runs, developers), so they never run by default. Use `make test-destructive`.
const destructiveEnv = "WHATFUNNEL_DESTRUCTIVE_TESTS"

func requireDestructive(t *testing.T) {
	t.Helper()
	if os.Getenv(destructiveEnv) != "1" {
		t.Skipf("destructive test: set %s=1 (or run `make test-destructive`) to stop/pause/kill dev stack containers", destructiveEnv)
	}
}

// composeCmd runs `docker compose <args>` from the repository root so it always
// targets the dev stack's docker-compose.yml regardless of the test's working dir.
func composeCmd(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)
	cmd.Dir = findRepoRoot(t)
	return cmd.CombinedOutput()
}

func redisReachable(addr string) bool {
	ps, err := pubsub.NewClient(addr) // pings with a bounded timeout
	if err != nil {
		return false
	}
	_ = ps.Close()
	return true
}

// cleanupAccountByEmail removes an account created through signup and all of its
// data using a fresh connection (the pool a test used before a database restart
// may be dead). It is safe to register before the account exists.
func cleanupAccountByEmail(t *testing.T, email string) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Errorf("cleanup %s: connect: %v", email, err)
		return
	}
	defer pool.Close()

	// Every table referencing accounts cascades, so deleting the account removes its data.
	if _, err := pool.Exec(ctx, `DELETE FROM accounts WHERE id IN (SELECT account_id FROM users WHERE email = $1)`, email); err != nil {
		t.Errorf("cleanup %s: delete account: %v", email, err)
	}
}
