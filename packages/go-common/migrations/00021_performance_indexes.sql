-- +goose Up
-- +goose StatementBegin

CREATE INDEX IF NOT EXISTS idx_messages_account_external_id
    ON messages (account_id, external_message_id)
    WHERE external_message_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_messages_account_provider_id
    ON messages (account_id, provider_message_id)
    WHERE provider_message_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_messages_convo_created_id_desc
    ON messages (conversation_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_messages_sender_user_id
    ON messages (sender_user_id)
    WHERE sender_user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_messages_reply_to_message_id
    ON messages (reply_to_message_id)
    WHERE reply_to_message_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ai_answer_events_account_message
    ON ai_answer_events (account_id, message_id);

CREATE INDEX IF NOT EXISTS idx_leads_account_current_state
    ON leads (account_id, current_state_key);

CREATE INDEX IF NOT EXISTS idx_kb_ingestions_status_created_at
    ON kb_ingestions (status, created_at)
    WHERE status IN ('queued', 'publishing');

CREATE INDEX IF NOT EXISTS idx_conversations_assigned_user_ids_gin
    ON conversations USING GIN (assigned_user_ids);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_conversations_assigned_user_ids_gin;
DROP INDEX IF EXISTS idx_kb_ingestions_status_created_at;
DROP INDEX IF EXISTS idx_leads_account_current_state;
DROP INDEX IF EXISTS idx_ai_answer_events_account_message;
DROP INDEX IF EXISTS idx_messages_reply_to_message_id;
DROP INDEX IF EXISTS idx_messages_sender_user_id;
DROP INDEX IF EXISTS idx_messages_convo_created_id_desc;
DROP INDEX IF EXISTS idx_messages_account_provider_id;
DROP INDEX IF EXISTS idx_messages_account_external_id;

-- +goose StatementEnd
