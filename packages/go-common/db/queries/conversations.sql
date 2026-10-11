-- name: UnassignUserFromConversations :exec
UPDATE conversations
SET assigned_user_ids = array_remove(assigned_user_ids, @user_id::uuid)
WHERE account_id = @account_id AND @user_id::uuid = ANY(assigned_user_ids);

-- name: GetConversationAssignedUsers :one
SELECT assigned_user_ids
FROM conversations
WHERE id = $1 AND account_id = $2;

-- name: GetConversationVisibility :one
-- Everything types.CanSeeConversation needs, in one round trip.
SELECT c.assigned_user_ids, a.settings
FROM conversations c
JOIN accounts a ON a.id = c.account_id
WHERE c.id = $1 AND c.account_id = $2;

-- name: GetConversationAccountAndAssignedUsers :one
SELECT account_id, assigned_user_ids
FROM conversations
WHERE id = $1;

