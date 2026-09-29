-- +goose Up
-- +goose StatementBegin

-- Outbound uploads are not attached to a message row (the message only
-- references the media id in its JSON content), so downloads could not be
-- authorised against the conversation. Record the conversation and uploader.
ALTER TABLE media_objects
    ADD COLUMN IF NOT EXISTS conversation_id UUID REFERENCES conversations(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS uploaded_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL;

-- Backfill existing outbound uploads from the message that references them.
UPDATE media_objects AS media
SET conversation_id = message.conversation_id
FROM messages AS message
WHERE media.conversation_id IS NULL
  AND media.message_id IS NULL
  AND message.account_id = media.account_id
  AND message.content->>'media_id' = media.id::TEXT;

CREATE INDEX IF NOT EXISTS idx_media_objects_conversation_id
    ON media_objects (conversation_id)
    WHERE conversation_id IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_media_objects_conversation_id;
ALTER TABLE media_objects
    DROP COLUMN IF EXISTS uploaded_by_user_id,
    DROP COLUMN IF EXISTS conversation_id;

-- +goose StatementEnd
