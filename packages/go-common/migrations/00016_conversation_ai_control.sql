-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS conversation_ai_state (
    conversation_id             UUID        PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE,
    account_id                  UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    state                       TEXT        NOT NULL DEFAULT 'active'
                                            CHECK (state IN (
                                                'active', 'paused_human', 'cooldown',
                                                'review_required', 'blocked_spam', 'blocked_manual'
                                            )),
    state_reason                TEXT,
    reply_override              TEXT        NOT NULL DEFAULT 'inherit'
                                            CHECK (reply_override IN ('inherit', 'enabled', 'disabled')),
    run_state                   TEXT        NOT NULL DEFAULT 'idle'
                                            CHECK (run_state IN ('idle', 'queued', 'replying')),
    run_started_at              TIMESTAMPTZ,
    generation_epoch            BIGINT      NOT NULL DEFAULT 0,
    cooldown_level              SMALLINT    NOT NULL DEFAULT 0 CHECK (cooldown_level BETWEEN 0 AND 4),
    next_review_at              TIMESTAMPTZ,
    unanswered_count            INTEGER     NOT NULL DEFAULT 0 CHECK (unanswered_count >= 0),
    unanswered_window_started_at TIMESTAMPTZ,
    last_acknowledgement_at     TIMESTAMPTZ,
    blocked_at                  TIMESTAMPTZ,
    version                     BIGINT      NOT NULL DEFAULT 0,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (account_id, conversation_id)
);

CREATE INDEX IF NOT EXISTS idx_conversation_ai_state_due
    ON conversation_ai_state (next_review_at)
    WHERE state = 'cooldown';

CREATE INDEX IF NOT EXISTS idx_conversation_ai_state_account_state
    ON conversation_ai_state (account_id, state);

CREATE TABLE IF NOT EXISTS conversation_ai_state_events (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id          UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    conversation_id     UUID        NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    actor_user_id       UUID                 REFERENCES users(id) ON DELETE SET NULL,
    from_state          TEXT,
    to_state            TEXT        NOT NULL,
    reason              TEXT,
    triggering_message_id UUID               REFERENCES messages(id) ON DELETE SET NULL,
    metadata            JSONB       NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_conversation_ai_state_events_conversation
    ON conversation_ai_state_events (conversation_id, created_at DESC);

INSERT INTO conversation_ai_state (conversation_id, account_id, state, state_reason, reply_override)
SELECT id, account_id,
       CASE WHEN ai_mode_active THEN 'active' ELSE 'paused_human' END,
       CASE WHEN ai_mode_active THEN NULL ELSE 'human_takeover' END,
       CASE
           WHEN ai_auto_reply_enabled IS TRUE THEN 'enabled'
           WHEN ai_auto_reply_enabled IS FALSE THEN 'disabled'
           ELSE 'inherit'
       END
FROM conversations
ON CONFLICT (conversation_id) DO NOTHING;

ALTER TABLE conversations
    DROP COLUMN ai_auto_reply_enabled,
    DROP COLUMN ai_mode_active;

ALTER TABLE messages ADD COLUMN idempotency_key TEXT;
CREATE UNIQUE INDEX idx_messages_account_idempotency_key
    ON messages (account_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

ALTER TABLE kb_concepts RENAME COLUMN body_markdown TO body_text;
ALTER TABLE patterns RENAME COLUMN answer_markdown TO answer_text;
ALTER TABLE kb_ingestion_items RENAME COLUMN body_markdown TO body_text;
ALTER TABLE kb_ingestion_patterns RENAME COLUMN answer_markdown TO answer_text;

CREATE OR REPLACE FUNCTION initialize_conversation_ai_state()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO conversation_ai_state (conversation_id, account_id)
    VALUES (NEW.id, NEW.account_id)
    ON CONFLICT (conversation_id) DO NOTHING;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS conversations_initialize_ai_state ON conversations;
CREATE TRIGGER conversations_initialize_ai_state
AFTER INSERT ON conversations
FOR EACH ROW EXECUTE FUNCTION initialize_conversation_ai_state();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS conversations_initialize_ai_state ON conversations;
DROP FUNCTION IF EXISTS initialize_conversation_ai_state();

ALTER TABLE conversations
    ADD COLUMN ai_mode_active BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN ai_auto_reply_enabled BOOLEAN;

DROP INDEX IF EXISTS idx_messages_account_idempotency_key;
ALTER TABLE messages DROP COLUMN idempotency_key;

ALTER TABLE kb_concepts RENAME COLUMN body_text TO body_markdown;
ALTER TABLE patterns RENAME COLUMN answer_text TO answer_markdown;
ALTER TABLE kb_ingestion_items RENAME COLUMN body_text TO body_markdown;
ALTER TABLE kb_ingestion_patterns RENAME COLUMN answer_text TO answer_markdown;

UPDATE conversations AS conversation
SET ai_mode_active = state.state = 'active',
    ai_auto_reply_enabled = CASE state.reply_override
        WHEN 'enabled' THEN TRUE
        WHEN 'disabled' THEN FALSE
        ELSE NULL
    END
FROM conversation_ai_state AS state
WHERE state.conversation_id = conversation.id;

DROP TABLE IF EXISTS conversation_ai_state_events;
DROP TABLE IF EXISTS conversation_ai_state;

-- +goose StatementEnd
