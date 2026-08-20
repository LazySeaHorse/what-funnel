-- +goose Up
-- +goose StatementBegin

-- A connection has a short-lived setup lifecycle that is distinct from the
-- long-lived messaging channel. Keeping it separate prevents credentials and
-- bridge-management details from leaking into the channel API.
ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_status_check;
ALTER TABLE channels
    ADD CONSTRAINT channels_status_check
    CHECK (status IN ('pending', 'connected', 'disconnected', 'error'));

CREATE TABLE IF NOT EXISTS channel_connections (
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

CREATE INDEX IF NOT EXISTS idx_channel_connections_account_id
    ON channel_connections(account_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_connections_active_platform
    ON channel_connections(account_id, platform)
    WHERE state NOT IN ('failed', 'cancelled');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS channel_connections;
UPDATE channels SET status = 'disconnected' WHERE status = 'pending';
ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_status_check;
ALTER TABLE channels
    ADD CONSTRAINT channels_status_check
    CHECK (status IN ('connected', 'disconnected', 'error'));
-- +goose StatementEnd
