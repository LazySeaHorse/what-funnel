-- +goose Up
-- +goose StatementBegin

-- Provider message mutations may enqueue more than one command for the same
-- local message. Command IDs, rather than message IDs, are the outbox identity.
ALTER TABLE message_outbox DROP CONSTRAINT message_outbox_message_id_key;
CREATE UNIQUE INDEX message_outbox_command_id_key ON message_outbox ((command->>'id'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS message_outbox_command_id_key;
ALTER TABLE message_outbox ADD CONSTRAINT message_outbox_message_id_key UNIQUE (message_id);

-- +goose StatementEnd
