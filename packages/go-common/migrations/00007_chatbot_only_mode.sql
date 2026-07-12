-- +goose Up
-- +goose StatementBegin
ALTER TABLE accounts ADD COLUMN product_mode TEXT NOT NULL DEFAULT 'full_workspace'
  CONSTRAINT check_product_mode CHECK (product_mode IN ('full_workspace', 'chatbot_only'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE accounts DROP COLUMN IF EXISTS product_mode;
-- +goose StatementEnd
