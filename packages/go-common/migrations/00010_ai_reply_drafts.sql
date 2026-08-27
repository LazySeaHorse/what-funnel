-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS ai_reply_drafts (
    id                UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id        UUID          NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    conversation_id   UUID          NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    source_message_id UUID          NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    draft_text        TEXT          NOT NULL,
    stage_matched     TEXT          NOT NULL CHECK (stage_matched IN ('pattern', 'embedding', 'llm_grounded')),
    confidence        NUMERIC,
    status            TEXT          NOT NULL DEFAULT 'pending'
                                      CHECK (status IN ('pending', 'used', 'dismissed', 'superseded')),
    used_message_id   UUID          REFERENCES messages(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_reply_drafts_one_pending_per_conversation
    ON ai_reply_drafts (account_id, conversation_id)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_ai_reply_drafts_conversation_created
    ON ai_reply_drafts (account_id, conversation_id, created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS ai_reply_drafts;

-- +goose StatementEnd
