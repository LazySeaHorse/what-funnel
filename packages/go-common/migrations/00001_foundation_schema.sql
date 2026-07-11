-- +goose Up
-- +goose StatementBegin

-- Enable pgvector extension. Later prompts depend on this already being
-- present; run it here so no subsequent migration needs to touch this layer.
CREATE EXTENSION IF NOT EXISTS vector;

-- enable gen_random_uuid() via pgcrypto (available in pgvector image)
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ============================================================
-- accounts
-- Root tenant boundary. One business per account.
-- ============================================================
CREATE TABLE IF NOT EXISTS accounts (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name               TEXT        NOT NULL,
    plan               TEXT        NOT NULL DEFAULT 'self_hosted',
    ai_provider_config TEXT,                    -- AES-256-GCM encrypted at application layer
    settings           JSONB       NOT NULL DEFAULT '{}',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_accounts_created_at ON accounts (created_at);

-- ============================================================
-- users
-- A person with login access to an account. Role: admin|member.
-- ============================================================
CREATE TABLE IF NOT EXISTS users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id    UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    email         TEXT        NOT NULL,
    password_hash TEXT        NOT NULL DEFAULT '',
    role          TEXT        NOT NULL CHECK (role IN ('admin', 'member')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (account_id, email)
);

CREATE INDEX IF NOT EXISTS idx_users_account_id ON users (account_id);
CREATE INDEX IF NOT EXISTS idx_users_email      ON users (email);

-- ============================================================
-- audit_logs
-- Every state-changing action writes a row here.
-- ============================================================
CREATE TABLE IF NOT EXISTS audit_logs (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id    UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    actor_user_id UUID                 REFERENCES users(id) ON DELETE SET NULL,
    action        TEXT        NOT NULL,
    target_type   TEXT        NOT NULL,
    target_id     UUID,
    metadata      JSONB       NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_account_id    ON audit_logs (account_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_user_id ON audit_logs (actor_user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at    ON audit_logs (created_at);

-- ============================================================
-- lead_pipelines
-- Admin-configured ordered list of lead states for an account.
-- states: JSONB array of {key, label, color} objects.
-- ============================================================
CREATE TABLE IF NOT EXISTS lead_pipelines (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name       TEXT        NOT NULL,
    states     JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lead_pipelines_account_id ON lead_pipelines (account_id);

-- ============================================================
-- invite_tokens
-- Pending user invites; email delivery is stubbed in v1.
-- ============================================================
CREATE TABLE IF NOT EXISTS invite_tokens (
    token      TEXT        PRIMARY KEY,
    account_id UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    email      TEXT        NOT NULL,
    role       TEXT        NOT NULL CHECK (role IN ('admin', 'member')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    used_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_invite_tokens_account_id ON invite_tokens (account_id);

-- ============================================================
-- sessions
-- Server-side session store for authboss.
-- ============================================================
CREATE TABLE IF NOT EXISTS sessions (
    token      TEXT        PRIMARY KEY,
    data       BYTEA       NOT NULL,
    expiry     TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions (expiry);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS invite_tokens;
DROP TABLE IF EXISTS lead_pipelines;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS accounts;
DROP EXTENSION IF EXISTS vector;
DROP EXTENSION IF EXISTS pgcrypto;
-- +goose StatementEnd
