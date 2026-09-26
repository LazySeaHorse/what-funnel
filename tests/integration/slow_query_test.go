package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSlowQueries verifies via pg_stat_statements that >= 99% of executed queries
// finish in under 50 ms mean execution time.
func TestSlowQueries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow query test in short mode")
	}

	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Ensure pg_stat_statements is available
	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements'
		);
	`).Scan(&exists)
	require.NoError(t, err)
	if !exists {
		t.Skip("pg_stat_statements extension not installed in database")
	}

	// Query total statements and count of those finishing under 50ms
	// Exclude CREATE DATABASE or admin utility maintenance if run in test suite
	var totalQueries, under50msQueries, over50msQueries int
	err = pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE mean_exec_time < 50.0),
			COUNT(*) FILTER (WHERE mean_exec_time >= 50.0)
		FROM pg_stat_statements
		WHERE query NOT LIKE '%CREATE DATABASE%';
	`).Scan(&totalQueries, &under50msQueries, &over50msQueries)
	require.NoError(t, err)

	if totalQueries == 0 {
		t.Skip("no query statements recorded yet in pg_stat_statements")
	}

	percentageUnder50ms := (float64(under50msQueries) / float64(totalQueries)) * 100.0
	t.Logf("pg_stat_statements: %d total queries, %d under 50ms (%.2f%%), %d over 50ms",
		totalQueries, under50msQueries, percentageUnder50ms, over50msQueries)

	// Log slow queries if any exist
	if over50msQueries > 0 {
		rows, err := pool.Query(ctx, `
			SELECT query, calls, mean_exec_time, max_exec_time
			FROM pg_stat_statements
			WHERE mean_exec_time >= 50.0 AND query NOT LIKE '%CREATE DATABASE%'
			ORDER BY mean_exec_time DESC
			LIMIT 5;
		`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var q string
				var calls int64
				var meanTime, maxTime float64
				if err := rows.Scan(&q, &calls, &meanTime, &maxTime); err == nil {
					t.Logf("Slow query (calls: %d, mean: %.2fms, max: %.2fms): %s", calls, meanTime, maxTime, q)
				}
			}
		}
	}

	assert.GreaterOrEqual(t, percentageUnder50ms, 99.0, "At least 99%% of queries must finish in under 50ms")
}
