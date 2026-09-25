-- name: GetAccountByID :one
SELECT id, name, plan, product_mode, settings, created_at
FROM accounts
WHERE id = $1;

-- name: GetAccountSettings :one
SELECT settings
FROM accounts
WHERE id = $1;

-- name: GetAccountSlug :one
SELECT slug
FROM accounts
WHERE id = $1;

-- name: CreateAccount :one
INSERT INTO accounts (name, plan, settings, product_mode)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: DeleteAccountByID :execresult
DELETE FROM accounts
WHERE id = $1;

-- name: UpdateAccountName :exec
UPDATE accounts
SET name = $1
WHERE id = $2;

-- name: UpdateAccountSettings :exec
UPDATE accounts
SET settings = $1
WHERE id = $2;

-- name: MergeAccountSettings :execresult
UPDATE accounts
SET settings = settings || @settings::jsonb
WHERE id = @id;

-- name: GetAccountProductModeForUpdate :one
SELECT product_mode
FROM accounts
WHERE id = $1
FOR UPDATE;

-- name: UpdateAccountProductMode :exec
UPDATE accounts
SET product_mode = @product_mode,
    settings = jsonb_set(COALESCE(settings, '{}'::jsonb), '{lead_tracking_enabled}', @lead_tracking_enabled::jsonb)
WHERE id = @id;

-- name: SetAccountSlug :exec
UPDATE accounts
SET slug = $1
WHERE id = $2;
