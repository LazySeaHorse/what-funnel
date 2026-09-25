-- +goose Up
-- +goose StatementBegin

-- HNSW indexes for cosine similarity search on embedding columns.
-- Without these, every <=> query is a full sequential scan of the table.
-- HNSW is preferred over IVFFlat here because it requires no upfront
-- cluster count tuning and degrades gracefully at small row counts.
--
-- m=16 / ef_construction=64 are the pgvector defaults and appropriate
-- for 1536-dimensional OpenAI/Gemini embeddings at this scale.
CREATE INDEX IF NOT EXISTS idx_patterns_embedding
    ON patterns USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

CREATE INDEX IF NOT EXISTS idx_kb_concepts_embedding
    ON kb_concepts USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_kb_concepts_embedding;
DROP INDEX IF EXISTS idx_patterns_embedding;

-- +goose StatementEnd
