-- +goose Up
-- +goose StatementBegin

-- 1. Add slug to accounts
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS slug TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_slug ON accounts (slug) WHERE slug IS NOT NULL;

-- 2. Update users table: username, nullable email, unique constraints, and updated role check
ALTER TABLE users ADD COLUMN IF NOT EXISTS username TEXT;
ALTER TABLE users ALTER COLUMN email DROP NOT NULL;

-- Update existing data if any
UPDATE users SET role = 'manager' WHERE role = 'admin';
UPDATE users SET role = 'agent' WHERE role = 'member';

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('manager', 'agent'));

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_account_id_username ON users (account_id, username) WHERE username IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_username ON users (username);

-- 3. Drop invite_tokens table
DROP TABLE IF EXISTS invite_tokens;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS invite_tokens (
    token      TEXT        PRIMARY KEY,
    account_id UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    email      TEXT        NOT NULL,
    role       TEXT        NOT NULL CHECK (role IN ('admin', 'member')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    used_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_invite_tokens_account_id ON invite_tokens (account_id);

DROP INDEX IF EXISTS idx_users_username;
DROP INDEX IF EXISTS idx_users_account_id_username;

UPDATE users SET role = 'admin' WHERE role = 'manager';
UPDATE users SET role = 'member' WHERE role = 'agent';

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'member'));

ALTER TABLE users DROP COLUMN IF EXISTS username;
DROP INDEX IF EXISTS idx_accounts_slug;
ALTER TABLE accounts DROP COLUMN IF EXISTS slug;

-- +goose StatementEnd
