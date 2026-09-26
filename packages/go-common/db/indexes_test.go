package db_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// expectedIndexes maps expected index names to their corresponding table names.
var expectedPerformanceIndexes = map[string]string{
	"idx_messages_account_external_id":        "messages",
	"idx_messages_account_provider_id":        "messages",
	"idx_messages_convo_created_id_desc":      "messages",
	"idx_messages_sender_user_id":             "messages",
	"idx_messages_reply_to_message_id":        "messages",
	"idx_ai_answer_events_account_message":    "ai_answer_events",
	"idx_leads_account_current_state":         "leads",
	"idx_kb_ingestions_status_created_at":     "kb_ingestions",
	"idx_conversations_assigned_user_ids_gin": "conversations",
}

// TestPerformanceIndexesMigrationFile validates that the migration file contains all required index definitions.
func TestPerformanceIndexesMigrationFile(t *testing.T) {
	// Find repo root or migration file relative to current directory
	migrationPath := filepath.Join("..", "migrations", "00021_performance_indexes.sql")
	content, err := os.ReadFile(migrationPath)
	if err != nil {
		// Try from repo root if running from different working directory
		migrationPath = filepath.Join("packages", "go-common", "migrations", "00021_performance_indexes.sql")
		content, err = os.ReadFile(migrationPath)
		require.NoError(t, err, "failed to read migration file 00021_performance_indexes.sql")
	}

	migrationText := string(content)
	assert.Contains(t, migrationText, "-- +goose Up")
	assert.Contains(t, migrationText, "-- +goose Down")

	for indexName, tableName := range expectedPerformanceIndexes {
		t.Run("migration_contains_"+indexName, func(t *testing.T) {
			assert.Contains(t, migrationText, indexName, "migration must reference index %s", indexName)
			assert.Contains(t, migrationText, tableName, "migration must reference table %s", tableName)
		})
	}
}

// TestPerformanceIndexesExistInDatabase verifies that all 9 performance indexes exist in Postgres schema.
func TestPerformanceIndexesExistInDatabase(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	rows, err := pool.Query(ctx, `
		SELECT tablename, indexname, indexdef
		FROM pg_indexes
		WHERE schemaname = 'public'
	`)
	require.NoError(t, err)
	defer rows.Close()

	foundIndexes := make(map[string]struct {
		table    string
		indexDef string
	})

	for rows.Next() {
		var table, index, indexDef string
		err := rows.Scan(&table, &index, &indexDef)
		require.NoError(t, err)
		foundIndexes[index] = struct {
			table    string
			indexDef string
		}{
			table:    table,
			indexDef: indexDef,
		}
	}
	require.NoError(t, rows.Err())

	for expectedIdx, expectedTable := range expectedPerformanceIndexes {
		t.Run("index_"+expectedIdx, func(t *testing.T) {
			info, exists := foundIndexes[expectedIdx]
			require.True(t, exists, "index %s must exist in public schema", expectedIdx)
			assert.Equal(t, expectedTable, info.table, "index %s should belong to table %s", expectedIdx, expectedTable)
		})
	}

	// Verify GIN index specifically uses gin access method
	ginInfo, exists := foundIndexes["idx_conversations_assigned_user_ids_gin"]
	require.True(t, exists)
	assert.True(t, strings.Contains(strings.ToLower(ginInfo.indexDef), "using gin"),
		"idx_conversations_assigned_user_ids_gin must be a GIN index: %s", ginInfo.indexDef)
}
