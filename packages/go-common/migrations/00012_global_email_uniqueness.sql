-- +goose Up
-- +goose StatementBegin

-- 1. Drop existing per-account composite unique constraint if present
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_account_id_email_key;

-- 2. Create global unique index on non-null non-empty email
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_global ON users (email) WHERE email IS NOT NULL AND email <> '';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_users_email_global;
ALTER TABLE users ADD CONSTRAINT users_account_id_email_key UNIQUE (account_id, email);

-- +goose StatementEnd
