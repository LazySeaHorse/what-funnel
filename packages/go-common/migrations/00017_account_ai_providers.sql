-- +goose Up
-- +goose StatementBegin

CREATE TABLE account_ai_providers (
    account_id        UUID        PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    base_url          TEXT        NOT NULL CHECK (btrim(base_url) <> ''),
    encrypted_api_key TEXT        NOT NULL CHECK (btrim(encrypted_api_key) <> ''),
    analysis_model    TEXT        NOT NULL CHECK (btrim(analysis_model) <> ''),
    reply_model       TEXT        NOT NULL CHECK (btrim(reply_model) <> ''),
    embedding_model   TEXT        NOT NULL CHECK (btrim(embedding_model) <> ''),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE accounts DROP COLUMN ai_provider_config;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE accounts ADD COLUMN ai_provider_config TEXT;
DROP TABLE account_ai_providers;

-- +goose StatementEnd
