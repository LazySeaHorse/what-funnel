-- +goose Up
-- +goose StatementBegin

-- ============================================================
-- channels
-- Connected messaging surface (e.g. Matrix bridged WhatsApp).
-- ============================================================
CREATE TABLE IF NOT EXISTS channels (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id         UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    type               TEXT        NOT NULL CHECK (type IN ('matrix_whatsapp', 'matrix_instagram', 'matrix_messenger', 'matrix_telegram', 'webchat')),
    bridge_identity    TEXT,
    bridge_credentials JSONB,                   -- encrypted at rest (stored as json with ciphertext)
    status             TEXT        NOT NULL DEFAULT 'disconnected' CHECK (status IN ('connected', 'disconnected', 'error')),
    status_detail      TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_channels_account_id ON channels (account_id);

-- ============================================================
-- contacts
-- People who have messaged the business on a channel.
-- ============================================================
CREATE TABLE IF NOT EXISTS contacts (
    id                     UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id             UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    channel_id             UUID        NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    external_identity      TEXT        NOT NULL,
    display_name           TEXT,
    avatar_url             TEXT,
    merged_into_contact_id UUID                 REFERENCES contacts(id) ON DELETE SET NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (channel_id, external_identity)
);

CREATE INDEX IF NOT EXISTS idx_contacts_account_id ON contacts (account_id);
CREATE INDEX IF NOT EXISTS idx_contacts_channel_id ON contacts (channel_id);

-- ============================================================
-- conversations
-- A thread with one contact on one channel.
-- ============================================================
CREATE TABLE IF NOT EXISTS conversations (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id         UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    contact_id         UUID        NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    channel_id         UUID        NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    status             TEXT        NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed')),
    assigned_user_ids  UUID[]      NOT NULL DEFAULT '{}',
    last_message_at    TIMESTAMPTZ,
    ai_mode_active     BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (contact_id, channel_id)
);

CREATE INDEX IF NOT EXISTS idx_conversations_account_id     ON conversations (account_id);
CREATE INDEX IF NOT EXISTS idx_conversations_last_message_at ON conversations (account_id, last_message_at);

-- ============================================================
-- messages
-- Individual messages inside a conversation.
-- ============================================================
CREATE TABLE IF NOT EXISTS messages (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id          UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    conversation_id     UUID        NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    direction           TEXT        NOT NULL CHECK (direction IN ('inbound', 'outbound')),
    sender_type         TEXT        NOT NULL CHECK (sender_type IN ('contact', 'human', 'ai')),
    sender_user_id      UUID                 REFERENCES users(id) ON DELETE SET NULL,
    content_type        TEXT        NOT NULL CHECK (content_type IN ('text', 'image', 'video', 'audio', 'document', 'reaction', 'location', 'contact')),
    content             JSONB       NOT NULL,
    external_message_id TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_messages_account_id ON messages (account_id);
CREATE INDEX IF NOT EXISTS idx_messages_chronological ON messages (conversation_id, created_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS conversations;
DROP TABLE IF EXISTS contacts;
DROP TABLE IF EXISTS channels;
-- +goose StatementEnd
