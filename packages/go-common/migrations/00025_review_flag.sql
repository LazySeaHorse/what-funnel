-- +goose Up
-- +goose StatementBegin

-- A soft review flag marks a conversation for a human WITHOUT changing the AI state: the AI stays
-- active and later messages are gated independently. Used for a single suspected prompt-injection
-- message and for a message the AI could not safely drop or answer. A human message or a manual
-- AI control change clears it.
ALTER TABLE conversation_ai_state
    ADD COLUMN IF NOT EXISTS review_flag_reason     TEXT,
    ADD COLUMN IF NOT EXISTS review_flag_priority   TEXT CHECK (review_flag_priority IN ('low', 'normal')),
    ADD COLUMN IF NOT EXISTS review_flag_message_id UUID REFERENCES messages(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS review_flagged_at      TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_conversation_ai_state_review_flag
    ON conversation_ai_state (account_id, review_flagged_at DESC)
    WHERE review_flag_reason IS NOT NULL;

-- New decision log fields: how the (three-valued) FAQ coverage was judged.
ALTER TABLE ai_router_decisions
    ADD COLUMN IF NOT EXISTS faq_coverage TEXT;
ALTER TABLE ai_router_decisions DROP COLUMN IF EXISTS faq_covers_everything;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE ai_router_decisions ADD COLUMN IF NOT EXISTS faq_covers_everything BOOLEAN;
UPDATE ai_router_decisions SET faq_covers_everything = (faq_coverage = 'full');
ALTER TABLE ai_router_decisions DROP COLUMN IF EXISTS faq_coverage;

DROP INDEX IF EXISTS idx_conversation_ai_state_review_flag;
ALTER TABLE conversation_ai_state
    DROP COLUMN IF EXISTS review_flagged_at,
    DROP COLUMN IF EXISTS review_flag_message_id,
    DROP COLUMN IF EXISTS review_flag_priority,
    DROP COLUMN IF EXISTS review_flag_reason;

-- +goose StatementEnd
