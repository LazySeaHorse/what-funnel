-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS kb_ingestion_patterns (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    ingestion_id       UUID        NOT NULL REFERENCES kb_ingestions(id) ON DELETE CASCADE,
    position           INT         NOT NULL,
    canonical_question TEXT        NOT NULL,
    answer_markdown    TEXT        NOT NULL,
    trigger_phrases    TEXT[]      NOT NULL DEFAULT '{}',
    status             TEXT        NOT NULL DEFAULT 'draft'
                                   CHECK (status IN ('draft', 'approved', 'rejected', 'published')),
    pattern_id         UUID        REFERENCES patterns(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (ingestion_id, position)
);

CREATE INDEX IF NOT EXISTS idx_kb_ingestion_patterns_ingestion
    ON kb_ingestion_patterns (ingestion_id, position);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS kb_ingestion_patterns;

-- +goose StatementEnd
