-- +goose Up
-- +goose StatementBegin

-- 1. Drop existing per-account composite unique constraint if present
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_account_id_email_key;

-- 2. Disambiguate any existing duplicate emails across accounts
UPDATE users u
SET email = u.email || '+dup-' || SUBSTRING(u.id::text, 1, 8)
WHERE u.email IS NOT NULL AND u.email <> ''
  AND u.id NOT IN (
      SELECT DISTINCT ON (email) id
      FROM users
      WHERE email IS NOT NULL AND email <> ''
      ORDER BY email, created_at DESC
  );

-- 3. Create global unique index on non-null non-empty email
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_global ON users (email) WHERE email IS NOT NULL AND email <> '';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_users_email_global;
ALTER TABLE users ADD CONSTRAINT users_account_id_email_key UNIQUE (account_id, email);

-- +goose StatementEnd
