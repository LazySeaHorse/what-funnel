-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS kb_ingestions (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id   UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    requested_by UUID        REFERENCES users(id) ON DELETE SET NULL,
    status       TEXT        NOT NULL DEFAULT 'queued'
                              CHECK (status IN ('queued', 'processing', 'review_required', 'publishing', 'complete', 'failed')),
    raw_text     TEXT        NOT NULL,
    error        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_ingestions_account_status
    ON kb_ingestions (account_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS kb_ingestion_items (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    ingestion_id  UUID        NOT NULL REFERENCES kb_ingestions(id) ON DELETE CASCADE,
    position      INT         NOT NULL,
    type          TEXT        NOT NULL,
    title         TEXT        NOT NULL,
    tags          TEXT[]      NOT NULL DEFAULT '{}',
    body_markdown TEXT        NOT NULL,
    status        TEXT        NOT NULL DEFAULT 'draft'
                               CHECK (status IN ('draft', 'approved', 'rejected', 'published')),
    concept_id    UUID        REFERENCES kb_concepts(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (ingestion_id, position)
);

CREATE INDEX IF NOT EXISTS idx_kb_ingestion_items_ingestion
    ON kb_ingestion_items (ingestion_id, position);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS kb_ingestion_items;
DROP TABLE IF EXISTS kb_ingestions;

-- +goose StatementEnd
