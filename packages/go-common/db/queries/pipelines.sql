-- name: ListPipelinesByAccount :many
SELECT id, account_id, name, states, created_at
FROM lead_pipelines
WHERE account_id = $1
ORDER BY created_at ASC;

-- name: GetPipelineStates :one
SELECT states
FROM lead_pipelines
WHERE id = $1 AND account_id = $2;

-- name: UpdatePipeline :exec
UPDATE lead_pipelines
SET name = $1, states = $2
WHERE id = $3 AND account_id = $4;

-- name: FindLeadsInStateKeys :many
SELECT id
FROM leads
WHERE account_id = @account_id AND current_state_key = ANY(@state_keys::text[]);
