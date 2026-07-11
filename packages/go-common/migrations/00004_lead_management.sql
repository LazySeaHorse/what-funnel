-- +goose Up
-- +goose StatementBegin

-- ============================================================
-- leads
-- Lead attached to a conversation.
-- ============================================================
CREATE TABLE IF NOT EXISTS leads (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id         UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    conversation_id    UUID        NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    pipeline_id        UUID        NOT NULL REFERENCES lead_pipelines(id) ON DELETE RESTRICT,
    current_state_key  TEXT        NOT NULL,
    tags               TEXT[]      NOT NULL DEFAULT '{}',
    created_by         UUID                 REFERENCES users(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (conversation_id)
);

CREATE INDEX IF NOT EXISTS idx_leads_account_id ON leads (account_id);
CREATE INDEX IF NOT EXISTS idx_leads_conversation_id ON leads (conversation_id);

-- ============================================================
-- lead_notes
-- Notes attached to a lead.
-- ============================================================
CREATE TABLE IF NOT EXISTS lead_notes (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id     UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    lead_id        UUID        NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    author_user_id UUID                 REFERENCES users(id) ON DELETE SET NULL,
    body           TEXT        NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lead_notes_account_id ON lead_notes (account_id);
CREATE INDEX IF NOT EXISTS idx_lead_notes_lead_created_at ON lead_notes (lead_id, created_at DESC);

-- ============================================================
-- lead_state_history
-- State transitions history for a lead.
-- ============================================================
CREATE TABLE IF NOT EXISTS lead_state_history (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    lead_id    UUID        NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    from_state TEXT,
    to_state   TEXT        NOT NULL,
    changed_by UUID                 REFERENCES users(id) ON DELETE SET NULL,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lead_state_history_account_id ON lead_state_history (account_id);
CREATE INDEX IF NOT EXISTS idx_lead_state_history_lead_id ON lead_state_history (lead_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS lead_state_history;
DROP TABLE IF EXISTS lead_notes;
DROP TABLE IF EXISTS leads;
-- +goose StatementEnd
