-- name: DeleteSessionsByUserID :exec
DELETE FROM sessions
WHERE convert_from(data, 'UTF8')::jsonb->>'user_id' = @user_id::text;

-- name: DeleteSessionsByAccountID :exec
DELETE FROM sessions
WHERE convert_from(data, 'UTF8')::jsonb->>'account_id' = @account_id::text;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions
WHERE expiry < NOW();
