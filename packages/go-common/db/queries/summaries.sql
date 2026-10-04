-- name: GetConversationSummary :one
-- Account-scoped read of the one summary row per conversation, with the live
-- message count so callers can tell whether newer messages exist.
SELECT s.summary_fields,
       s.generated_at,
       s.message_count_at_generation,
       (SELECT COUNT(*) FROM messages m
         WHERE m.conversation_id = s.conversation_id AND m.account_id = s.account_id)::int AS current_message_count
FROM conversation_summaries s
WHERE s.conversation_id = $1 AND s.account_id = $2;
