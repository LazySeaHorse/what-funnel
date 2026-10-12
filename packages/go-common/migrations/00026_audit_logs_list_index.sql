-- +goose Up
-- +goose StatementBegin

-- Serves the audit log list: newest first per account with keyset pagination on (created_at, id).
CREATE INDEX IF NOT EXISTS idx_audit_logs_account_created_id
    ON audit_logs (account_id, created_at DESC, id DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_audit_logs_account_created_id;

-- +goose StatementEnd
