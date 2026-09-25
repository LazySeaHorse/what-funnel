-- name: GetMessageForNotification :one
SELECT id, account_id, conversation_id, direction, sender_type, sender_user_id, content_type, content, external_message_id, created_at
FROM messages
WHERE id = $1 AND account_id = $2;
