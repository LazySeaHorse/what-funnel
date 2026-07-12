-- +goose Up
-- +goose StatementBegin

-- ============================================================
-- ai_answer_events
-- ============================================================
CREATE TABLE IF NOT EXISTS ai_answer_events (
    id               UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id       UUID          NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    conversation_id  UUID          NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    message_id       UUID          NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    stage_matched    TEXT          NOT NULL CHECK (stage_matched IN ('pattern', 'embedding', 'llm_grounded', 'none')),
    confidence       NUMERIC,
    action           TEXT          NOT NULL CHECK (action IN ('auto_sent', 'drafted', 'flagged_human')),
    reply_message_id UUID          REFERENCES messages(id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ai_answer_events_account_id ON ai_answer_events (account_id);
CREATE INDEX IF NOT EXISTS idx_ai_answer_events_conversation_id ON ai_answer_events (conversation_id);

-- ============================================================
-- conversation_summaries
-- ============================================================
CREATE TABLE IF NOT EXISTS conversation_summaries (
    id                          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id                  UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    conversation_id             UUID        NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    summary_fields              JSONB       NOT NULL DEFAULT '{}',
    generated_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    message_count_at_generation INT         NOT NULL,
    UNIQUE (conversation_id)
);

CREATE INDEX IF NOT EXISTS idx_conversation_summaries_account_id ON conversation_summaries (account_id);

-- ============================================================
-- users
-- ============================================================
ALTER TABLE users ADD COLUMN IF NOT EXISTS reply_mode_override TEXT CHECK (reply_mode_override IN ('auto_send', 'draft_only'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN IF EXISTS reply_mode_override;
DROP TABLE IF EXISTS conversation_summaries;
DROP TABLE IF EXISTS ai_answer_events;
-- +goose StatementEnd
