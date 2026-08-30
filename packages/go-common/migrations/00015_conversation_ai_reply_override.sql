-- +goose Up
-- +goose StatementBegin

ALTER TABLE conversations
    ADD COLUMN IF NOT EXISTS ai_auto_reply_enabled BOOLEAN;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE conversations
    DROP COLUMN IF EXISTS ai_auto_reply_enabled;

-- +goose StatementEnd
