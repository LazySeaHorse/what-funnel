-- +goose Up
-- +goose StatementBegin

-- ============================================================
-- kb_concepts
-- ============================================================
CREATE TABLE IF NOT EXISTS kb_concepts (
    id            UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id    UUID          NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    slug          TEXT          NOT NULL,
    type          TEXT          NOT NULL, -- e.g. faq, policy, hours, service, pricing
    title         TEXT          NOT NULL,
    tags          TEXT[]        NOT NULL DEFAULT '{}',
    body_markdown TEXT          NOT NULL,
    embedding     VECTOR(1536),
    source        TEXT          NOT NULL CHECK (source IN ('owner_pasted', 'ai_compiled')),
    created_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE (account_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_kb_concepts_account_id ON kb_concepts (account_id);

-- ============================================================
-- patterns
-- ============================================================
CREATE TABLE IF NOT EXISTS patterns (
    id                 UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id         UUID          NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    trigger_phrases    TEXT[]        NOT NULL,
    canonical_question TEXT          NOT NULL,
    answer_markdown    TEXT          NOT NULL,
    embedding          VECTOR(1536),
    created_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_patterns_account_id ON patterns (account_id);

-- ============================================================
-- automation_suggestions
-- ============================================================
CREATE TABLE IF NOT EXISTS automation_suggestions (
    id                 UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id         UUID          NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    type               TEXT          NOT NULL CHECK (type IN ('new_pattern', 'new_kb_concept', 'edited_answer')),
    source_message_ids UUID[]        NOT NULL DEFAULT '{}',
    proposed_payload   JSONB         NOT NULL,
    confidence         NUMERIC,
    status             TEXT          NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'edited')),
    reviewed_by        UUID          REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at        TIMESTAMPTZ,
    created_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_automation_suggestions_account_id ON automation_suggestions (account_id);

-- ============================================================
-- kb_mining_runs
-- ============================================================
CREATE TABLE IF NOT EXISTS kb_mining_runs (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id          UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    run_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    window_start        TIMESTAMPTZ NOT NULL,
    window_end          TIMESTAMPTZ NOT NULL,
    messages_scanned    INT         NOT NULL,
    clusters_found      INT         NOT NULL,
    suggestions_created INT         NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_kb_mining_runs_account_id_run_at ON kb_mining_runs (account_id, run_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS kb_mining_runs;
DROP TABLE IF EXISTS automation_suggestions;
DROP TABLE IF EXISTS patterns;
DROP TABLE IF EXISTS kb_concepts;
-- +goose StatementEnd
