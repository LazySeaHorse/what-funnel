-- +goose Up
-- +goose StatementBegin

-- convert_from() is only STABLE, so it cannot appear in an index expression.
-- session_data_json wraps it in an IMMUTABLE function (session data is always
-- UTF-8 JSON) so revocation lookups by user/account can use an index.
CREATE OR REPLACE FUNCTION session_data_json(data BYTEA) RETURNS JSONB
    LANGUAGE SQL IMMUTABLE STRICT PARALLEL SAFE
    AS $$ SELECT convert_from(data, 'UTF8')::jsonb $$;

CREATE INDEX IF NOT EXISTS idx_sessions_user_id
    ON sessions ((session_data_json(data)->>'user_id'));

CREATE INDEX IF NOT EXISTS idx_sessions_account_id
    ON sessions ((session_data_json(data)->>'account_id'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_sessions_account_id;
DROP INDEX IF EXISTS idx_sessions_user_id;
DROP FUNCTION IF EXISTS session_data_json(BYTEA);
-- +goose StatementEnd
