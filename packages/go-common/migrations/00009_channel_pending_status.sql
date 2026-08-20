-- +goose Up
-- +goose StatementBegin
ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_status_check;
ALTER TABLE channels
    ADD CONSTRAINT channels_status_check
    CHECK (status IN ('pending', 'connected', 'disconnected', 'error'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE channels SET status = 'disconnected' WHERE status = 'pending';
ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_status_check;
ALTER TABLE channels
    ADD CONSTRAINT channels_status_check
    CHECK (status IN ('connected', 'disconnected', 'error'));
-- +goose StatementEnd
