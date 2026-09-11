-- +goose Up
-- +goose StatementBegin

-- There are no production users at this rewrite boundary, so legacy bridge
-- channels are deliberately discarded instead of migrated.
DELETE FROM channels WHERE type LIKE 'matrix_%';
DROP TABLE IF EXISTS channel_connections;
ALTER TABLE channels
    DROP COLUMN IF EXISTS bridge_credentials,
    DROP COLUMN IF EXISTS bridge_identity;

ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_type_check;
ALTER TABLE channels
    ADD CONSTRAINT channels_type_check
    CHECK (type IN ('whatsapp', 'telegram', 'webchat'));

ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_status_check;
ALTER TABLE channels
    ADD CONSTRAINT channels_status_check
    CHECK (status IN (
        'pending', 'awaiting_scan', 'connecting', 'connected', 'disconnected', 'error'
    ));

ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_content_type_check;
UPDATE messages
SET content_type = 'notice',
    content = '{"text":"Open the original platform to view this unsupported message.","notice_code":"unsupported"}'::JSONB
WHERE content_type IN ('reaction', 'location', 'contact');
ALTER TABLE messages
    ADD CONSTRAINT messages_content_type_check
    CHECK (content_type IN ('text', 'image', 'video', 'audio', 'document', 'notice'));

ALTER TABLE channels
    ADD COLUMN label TEXT,
    ADD COLUMN provider TEXT,
    ADD COLUMN remote_account_id TEXT,
    ADD COLUMN capabilities JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

UPDATE channels
SET provider = type,
label = INITCAP(type)
WHERE provider IS NULL OR label IS NULL;

CREATE UNIQUE INDEX idx_channels_account_provider_label
    ON channels (account_id, provider, LOWER(label))
    WHERE provider IS NOT NULL AND label IS NOT NULL;

-- A provider connection is the control-plane state for one adapter-owned
-- account. Credentials and protocol session bytes never enter PostgreSQL.
CREATE TABLE provider_connections (
    channel_id         UUID PRIMARY KEY REFERENCES channels(id) ON DELETE CASCADE,
    account_id         UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    provider           TEXT        NOT NULL CHECK (provider IN ('whatsapp', 'telegram')),
    state              TEXT        NOT NULL CHECK (state IN (
        'pending', 'awaiting_scan', 'connecting', 'connected', 'disconnected', 'error'
    )),
    detail             TEXT,
    remote_account_id  TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (account_id, channel_id)
);

CREATE INDEX idx_provider_connections_account_id
    ON provider_connections (account_id, created_at DESC);

ALTER TABLE conversations ADD COLUMN external_thread_id TEXT;
UPDATE conversations
SET external_thread_id = contacts.external_identity
FROM contacts
WHERE contacts.id = conversations.contact_id
  AND conversations.external_thread_id IS NULL;
CREATE UNIQUE INDEX idx_conversations_channel_external_thread
    ON conversations (channel_id, external_thread_id)
    WHERE external_thread_id IS NOT NULL;

ALTER TABLE messages
    ADD COLUMN provider_message_id TEXT,
    ADD COLUMN reply_to_message_id UUID REFERENCES messages(id) ON DELETE SET NULL,
    ADD COLUMN delivery_status TEXT NOT NULL DEFAULT 'sent'
        CHECK (delivery_status IN ('queued', 'sent', 'delivered', 'read', 'failed')),
    ADD COLUMN delivery_detail TEXT,
    ADD COLUMN provider_timestamp TIMESTAMPTZ,
    ADD COLUMN edited_at TIMESTAMPTZ,
    ADD COLUMN deleted_at TIMESTAMPTZ;

UPDATE messages
SET provider_message_id = external_message_id,
    provider_timestamp = created_at
WHERE provider_message_id IS NULL OR provider_timestamp IS NULL;

CREATE UNIQUE INDEX idx_messages_conversation_provider_message
    ON messages (conversation_id, provider_message_id)
    WHERE provider_message_id IS NOT NULL;

CREATE TABLE message_reactions (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id            UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    message_id            UUID        NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    sender_external_id    TEXT        NOT NULL,
    emoji                 TEXT        NOT NULL,
    provider_timestamp    TIMESTAMPTZ NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (message_id, sender_external_id)
);

CREATE INDEX idx_message_reactions_account_id
    ON message_reactions (account_id);

CREATE TABLE media_objects (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id            UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    message_id            UUID                 REFERENCES messages(id) ON DELETE CASCADE,
    channel_id            UUID        NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    provider_ref          TEXT,
    filename              TEXT,
    mime_type             TEXT        NOT NULL,
    size_bytes            BIGINT      NOT NULL CHECK (size_bytes BETWEEN 0 AND 20971520),
    storage_key           TEXT,
    expires_at            TIMESTAMPTZ NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CHECK (storage_key IS NOT NULL OR provider_ref IS NOT NULL)
);

CREATE INDEX idx_media_objects_account_id ON media_objects (account_id);
CREATE INDEX idx_media_objects_expiry ON media_objects (expires_at);

CREATE TABLE message_outbox (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id      UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    channel_id      UUID        NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    message_id      UUID        NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    provider        TEXT        NOT NULL CHECK (provider IN ('whatsapp', 'telegram')),
    command         JSONB       NOT NULL,
    attempts        INTEGER     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_at      TIMESTAMPTZ,
    claimed_by      TEXT,
    dispatched_at   TIMESTAMPTZ,
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (message_id)
);

CREATE INDEX idx_message_outbox_ready
    ON message_outbox (available_at, created_at)
    WHERE dispatched_at IS NULL;

CREATE TABLE processed_adapter_events (
    provider       TEXT        NOT NULL CHECK (provider IN ('whatsapp', 'telegram')),
    event_id       TEXT        NOT NULL,
    channel_id     UUID        NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    processed_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (provider, event_id)
);

CREATE INDEX idx_processed_adapter_events_processed_at
    ON processed_adapter_events (processed_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS processed_adapter_events;
DROP TABLE IF EXISTS message_outbox;
DROP TABLE IF EXISTS media_objects;
DROP TABLE IF EXISTS message_reactions;

DROP INDEX IF EXISTS idx_messages_conversation_provider_message;
ALTER TABLE messages
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS edited_at,
    DROP COLUMN IF EXISTS provider_timestamp,
    DROP COLUMN IF EXISTS delivery_detail,
    DROP COLUMN IF EXISTS delivery_status,
    DROP COLUMN IF EXISTS reply_to_message_id,
    DROP COLUMN IF EXISTS provider_message_id;

DROP INDEX IF EXISTS idx_conversations_channel_external_thread;
ALTER TABLE conversations DROP COLUMN IF EXISTS external_thread_id;

DROP TABLE IF EXISTS provider_connections;
DROP INDEX IF EXISTS idx_channels_account_provider_label;
ALTER TABLE channels
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS capabilities,
    DROP COLUMN IF EXISTS remote_account_id,
    DROP COLUMN IF EXISTS provider,
    DROP COLUMN IF EXISTS label;

ALTER TABLE channels
    ADD COLUMN bridge_identity TEXT,
    ADD COLUMN bridge_credentials JSONB;

CREATE TABLE channel_connections (
    channel_id          UUID PRIMARY KEY REFERENCES channels(id) ON DELETE CASCADE,
    account_id          UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    platform            TEXT NOT NULL,
    bridge_identity     TEXT NOT NULL,
    management_room_id  TEXT,
    state               TEXT NOT NULL CHECK (state IN (
        'awaiting_scan', 'awaiting_phone', 'awaiting_code', 'awaiting_session',
        'connecting', 'connected', 'failed', 'cancelled'
    )),
    detail              TEXT,
    last_event_id       TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_channel_connections_account_id ON channel_connections(account_id);
CREATE UNIQUE INDEX idx_channel_connections_active_platform
    ON channel_connections(account_id, platform)
    WHERE state NOT IN ('failed', 'cancelled');

ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_type_check;
ALTER TABLE channels
    ADD CONSTRAINT channels_type_check
    CHECK (type IN ('matrix_whatsapp', 'matrix_instagram', 'matrix_messenger', 'matrix_telegram', 'webchat'));

ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_status_check;
ALTER TABLE channels
    ADD CONSTRAINT channels_status_check
    CHECK (status IN ('pending', 'connected', 'disconnected', 'error'));

ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_content_type_check;
ALTER TABLE messages
    ADD CONSTRAINT messages_content_type_check
    CHECK (content_type IN ('text', 'image', 'video', 'audio', 'document', 'reaction', 'location', 'contact'));

-- +goose StatementEnd
