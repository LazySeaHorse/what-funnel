-- name: ListUsersByAccount :many
SELECT id, account_id, COALESCE(email, '') AS email, COALESCE(username, '') AS username, role, created_at
FROM users
WHERE account_id = $1
ORDER BY created_at ASC;

-- name: GetUserByID :one
SELECT id, account_id, COALESCE(email, '') AS email, COALESCE(username, '') AS username, role, created_at
FROM users
WHERE id = $1 AND account_id = $2;

-- name: GetUserByEmail :one
SELECT id, account_id, COALESCE(email, '') AS email, COALESCE(username, '') AS username, password_hash, role, created_at
FROM users
WHERE email = @email::text
LIMIT 1;

-- name: GetUserBySlugIdentifier :one
SELECT u.id, u.account_id, COALESCE(u.email, '') AS email, COALESCE(u.username, '') AS username, u.password_hash, u.role, u.created_at
FROM users u
JOIN accounts a ON a.id = u.account_id
WHERE (a.slug || '-' || u.username) = @identifier::text
LIMIT 1;

-- name: CountUsersByEmail :one
SELECT COUNT(*)
FROM users
WHERE email = @email::text;

-- name: CheckUserExistsInAccount :one
SELECT EXISTS(
    SELECT 1 FROM users WHERE id = $1 AND account_id = $2
);

-- name: CreateUser :one
INSERT INTO users (account_id, email, username, password_hash, role)
VALUES (@account_id, NULLIF(@email::text, ''), NULLIF(@username::text, ''), @password_hash, @role)
RETURNING id, account_id, COALESCE(email, '') AS email, COALESCE(username, '') AS username, role, created_at;

-- name: CreateUserWithExplicitID :exec
INSERT INTO users (id, account_id, email, username, password_hash, role, created_at)
VALUES (@id, @account_id, NULLIF(@email::text, ''), NULLIF(@username::text, ''), @password_hash, @role, @created_at);

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = $1
WHERE id = $2 AND account_id = $3;

-- name: UpdateUserPasswordGlobal :exec
UPDATE users
SET password_hash = $1
WHERE id = $2;

-- name: UpdateUserRole :exec
UPDATE users
SET role = $1
WHERE id = $2 AND account_id = $3;

-- name: DeleteUserFromAccount :exec
DELETE FROM users
WHERE id = $1 AND account_id = $2;

-- name: CountUserInAccount :one
SELECT COUNT(*)
FROM users
WHERE id = $1 AND account_id = $2;

-- name: GetUserReplyMode :one
SELECT u.reply_mode_override, a.settings
FROM users u
JOIN accounts a ON a.id = u.account_id
WHERE u.id = $1 AND u.account_id = $2;

-- name: UpdateUserReplyMode :exec
UPDATE users
SET reply_mode_override = $1
WHERE id = $2 AND account_id = $3;
