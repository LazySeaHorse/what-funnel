-- name: DeleteSessionsByUserID :exec
DELETE FROM sessions
WHERE session_data_json(data)->>'user_id' = @user_id::text;

-- name: DeleteSessionsByAccountID :exec
DELETE FROM sessions
WHERE session_data_json(data)->>'account_id' = @account_id::text;
