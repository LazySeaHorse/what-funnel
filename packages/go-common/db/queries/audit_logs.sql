-- name: ListAuditLogs :many
-- Newest first with keyset pagination: pass the previous page's last (created_at, id) as the cursor.
SELECT a.id, a.actor_user_id, u.username AS actor_name, u.email AS actor_email,
       a.action, a.target_type, a.target_id, a.metadata, a.created_at
FROM audit_logs a
LEFT JOIN users u ON u.id = a.actor_user_id
WHERE a.account_id = @account_id
  AND (sqlc.narg('actor_user_id')::uuid IS NULL OR a.actor_user_id = sqlc.narg('actor_user_id')::uuid)
  AND (sqlc.narg('action')::text IS NULL OR a.action = sqlc.narg('action')::text)
  AND (sqlc.narg('from_ts')::timestamptz IS NULL OR a.created_at >= sqlc.narg('from_ts')::timestamptz)
  AND (sqlc.narg('to_ts')::timestamptz IS NULL OR a.created_at <= sqlc.narg('to_ts')::timestamptz)
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
       OR (a.created_at, a.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY a.created_at DESC, a.id DESC
LIMIT @row_limit;
