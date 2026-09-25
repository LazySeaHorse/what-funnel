-- name: UnassignUserFromConversations :exec
UPDATE conversations
SET assigned_user_ids = array_remove(assigned_user_ids, @user_id::uuid)
WHERE account_id = @account_id AND @user_id::uuid = ANY(assigned_user_ids);

-- name: GetConversationAssignedUsers :one
SELECT assigned_user_ids
FROM conversations
WHERE id = $1 AND account_id = $2;

-- name: GetConversationAccountAndAssignedUsers :one
SELECT account_id, assigned_user_ids
FROM conversations
WHERE id = $1;

