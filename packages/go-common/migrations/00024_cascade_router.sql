-- +goose Up
-- +goose StatementBegin

-- FAQs (patterns) are no longer matched by fuzzy tokens or embeddings. The router LLM sees
-- the account's approved FAQs as a menu, so the embedding column and its index are dropped.
DROP INDEX IF EXISTS idx_patterns_embedding;
ALTER TABLE patterns DROP COLUMN IF EXISTS embedding;

-- not_for: optional boundary note shown to the router ("not for: pricing questions").
-- approved_at: set only by the human approval flows (suggestion approval, ingestion publish,
-- owner edit in the knowledge panel). The cascade only uses FAQs with approved_at IS NOT NULL,
-- so a writer that forgets to approve produces an unused FAQ instead of an auto-sent one.
ALTER TABLE patterns
    ADD COLUMN IF NOT EXISTS not_for     TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS approved_by UUID REFERENCES users(id) ON DELETE SET NULL;

-- Every existing FAQ was created through a review flow or by the owner.
UPDATE patterns SET approved_at = created_at WHERE approved_at IS NULL;

ALTER TABLE kb_ingestion_patterns
    ADD COLUMN IF NOT EXISTS not_for TEXT NOT NULL DEFAULT '';

-- New answer stages: greeting (canned first-message greeting), canned (approved FAQ),
-- rag (grounded KB answer), handoff (escalated or failed closed), ignored (pure acknowledgement).
ALTER TABLE ai_answer_events DROP CONSTRAINT IF EXISTS ai_answer_events_stage_matched_check;
ALTER TABLE ai_answer_events DROP CONSTRAINT IF EXISTS ai_answer_events_action_check;
UPDATE ai_answer_events SET stage_matched = 'canned' WHERE stage_matched IN ('pattern', 'embedding');
UPDATE ai_answer_events SET stage_matched = 'rag' WHERE stage_matched = 'llm_grounded';
ALTER TABLE ai_answer_events
    ADD CONSTRAINT ai_answer_events_stage_matched_check
        CHECK (stage_matched IN ('greeting', 'canned', 'rag', 'handoff', 'ignored', 'none')),
    ADD CONSTRAINT ai_answer_events_action_check
        CHECK (action IN ('auto_sent', 'drafted', 'flagged_human', 'no_reply'));

ALTER TABLE ai_reply_drafts DROP CONSTRAINT IF EXISTS ai_reply_drafts_stage_matched_check;
UPDATE ai_reply_drafts SET stage_matched = 'canned' WHERE stage_matched IN ('pattern', 'embedding');
UPDATE ai_reply_drafts SET stage_matched = 'rag' WHERE stage_matched = 'llm_grounded';
ALTER TABLE ai_reply_drafts
    ADD CONSTRAINT ai_reply_drafts_stage_matched_check
        CHECK (stage_matched IN ('greeting', 'canned', 'rag'));

-- One row per router decision, for offline evaluation and active learning.
CREATE TABLE IF NOT EXISTS ai_router_decisions (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id            UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    conversation_id       UUID        NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    message_id            UUID        NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    prompt_version        TEXT        NOT NULL,
    model                 TEXT        NOT NULL DEFAULT '',
    bubble_count          INT         NOT NULL DEFAULT 1,
    route                 TEXT,
    faq_id                UUID,
    faq_covers_everything BOOLEAN,
    handoff_reason        TEXT,
    outcome               TEXT        NOT NULL,
    outcome_detail        TEXT        NOT NULL DEFAULT '',
    latency_ms            INT,
    prompt_tokens         INT,
    completion_tokens     INT,
    cached_tokens         INT,
    error                 TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ai_router_decisions_account_created
    ON ai_router_decisions (account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_router_decisions_conversation
    ON ai_router_decisions (conversation_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS ai_router_decisions;

ALTER TABLE ai_reply_drafts DROP CONSTRAINT IF EXISTS ai_reply_drafts_stage_matched_check;
UPDATE ai_reply_drafts SET stage_matched = 'pattern' WHERE stage_matched IN ('canned', 'greeting');
UPDATE ai_reply_drafts SET stage_matched = 'llm_grounded' WHERE stage_matched = 'rag';
ALTER TABLE ai_reply_drafts
    ADD CONSTRAINT ai_reply_drafts_stage_matched_check
        CHECK (stage_matched IN ('pattern', 'embedding', 'llm_grounded'));

ALTER TABLE ai_answer_events DROP CONSTRAINT IF EXISTS ai_answer_events_stage_matched_check;
ALTER TABLE ai_answer_events DROP CONSTRAINT IF EXISTS ai_answer_events_action_check;
UPDATE ai_answer_events SET stage_matched = 'pattern' WHERE stage_matched IN ('canned', 'greeting');
UPDATE ai_answer_events SET stage_matched = 'llm_grounded' WHERE stage_matched = 'rag';
UPDATE ai_answer_events SET stage_matched = 'none' WHERE stage_matched IN ('handoff', 'ignored');
UPDATE ai_answer_events SET action = 'flagged_human' WHERE action = 'no_reply';
ALTER TABLE ai_answer_events
    ADD CONSTRAINT ai_answer_events_stage_matched_check
        CHECK (stage_matched IN ('pattern', 'embedding', 'llm_grounded', 'none')),
    ADD CONSTRAINT ai_answer_events_action_check
        CHECK (action IN ('auto_sent', 'drafted', 'flagged_human'));

ALTER TABLE kb_ingestion_patterns DROP COLUMN IF EXISTS not_for;
ALTER TABLE patterns
    DROP COLUMN IF EXISTS approved_by,
    DROP COLUMN IF EXISTS approved_at,
    DROP COLUMN IF EXISTS not_for;
ALTER TABLE patterns ADD COLUMN IF NOT EXISTS embedding VECTOR(1536);
CREATE INDEX IF NOT EXISTS idx_patterns_embedding
    ON patterns USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- +goose StatementEnd
