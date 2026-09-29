package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSlowQueries drives a representative API workload and verifies via
// pg_stat_statements that >= 99% of the statements it executed finish in under
// 50 ms mean execution time.
//
// pg_stat_statements must be preloaded (docker-compose.yml starts postgres with
// shared_preload_libraries=pg_stat_statements; recreate the postgres container
// after pulling that change). The statistics are reset at the start so the
// measurement covers this test's workload rather than whatever ran earlier.
// Other clients of the same database running concurrently can still add
// statements, so this is a smoke check for slow queries, not a benchmark.
func TestSlowQueries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow query test in short mode")
	}
	skipIfServicesDown(t)

	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS pg_stat_statements`); err != nil {
		t.Fatalf("create pg_stat_statements extension: %v", err)
	}
	if _, err := pool.Exec(ctx, `SELECT pg_stat_statements_reset()`); err != nil {
		if strings.Contains(err.Error(), "shared_preload_libraries") {
			// Recreate the postgres container so docker-compose.yml's command takes effect.
			skipOrFail(t, "pg_stat_statements is not preloaded; recreate postgres with the current docker-compose.yml: %v", err)
		}
		t.Fatalf("reset pg_stat_statements: %v", err)
	}

	// Workload: signup, session lookup, and the read endpoints the inbox loads.
	email := uniqueEmail("slowq")
	t.Cleanup(func() { cleanupAccountByEmail(t, email) })
	client := newClient()
	resp, _ := post(t, client, gatewayURL+"/auth/signup", map[string]string{
		"account_name": "E2E Slow Query Co",
		"email":        email,
		"password":     "AdminPassword123!",
	})
	require.Equal(t, 201, resp.StatusCode)
	for i := 0; i < 5; i++ {
		for _, path := range []string{"/auth/me", "/workspace/account", "/workspace/pipelines", "/conversations", "/workspace/users"} {
			r, _ := get(t, client, gatewayURL+path)
			require.Less(t, r.StatusCode, 500, "GET %s must not fail", path)
		}
	}

	var totalQueries, under50msQueries, over50msQueries int
	err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE mean_exec_time < 50.0),
			COUNT(*) FILTER (WHERE mean_exec_time >= 50.0)
		FROM pg_stat_statements
		WHERE query NOT ILIKE '%pg_stat_statements%'
		  AND query NOT ILIKE '%CREATE DATABASE%'
		  AND query NOT ILIKE 'CREATE EXTENSION%';
	`).Scan(&totalQueries, &under50msQueries, &over50msQueries)
	require.NoError(t, err)
	require.NotZero(t, totalQueries, "the workload must have recorded statements in pg_stat_statements")

	percentageUnder50ms := (float64(under50msQueries) / float64(totalQueries)) * 100.0
	t.Logf("pg_stat_statements: %d total queries, %d under 50ms (%.2f%%), %d over 50ms",
		totalQueries, under50msQueries, percentageUnder50ms, over50msQueries)

	if over50msQueries > 0 {
		rows, err := pool.Query(ctx, `
			SELECT query, calls, mean_exec_time, max_exec_time
			FROM pg_stat_statements
			WHERE mean_exec_time >= 50.0 AND query NOT ILIKE '%pg_stat_statements%'
			ORDER BY mean_exec_time DESC
			LIMIT 5;
		`)
		require.NoError(t, err)
		defer rows.Close()
		for rows.Next() {
			var q string
			var calls int64
			var meanTime, maxTime float64
			require.NoError(t, rows.Scan(&q, &calls, &meanTime, &maxTime))
			t.Logf("Slow query (calls: %d, mean: %.2fms, max: %.2fms): %s", calls, meanTime, maxTime, q)
		}
		require.NoError(t, rows.Err())
	}

	assert.GreaterOrEqual(t, percentageUnder50ms, 99.0, "At least 99%% of queries must finish in under 50ms")
}
