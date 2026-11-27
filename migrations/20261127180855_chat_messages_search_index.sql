-- +goose NO TRANSACTION
-- +goose Up
CREATE INDEX CONCURRENTLY idx_chat_messages_search ON chat_messages
    USING gin (public.search_text(body) public.gin_trgm_ops)
    WHERE body IS NOT NULL AND deleted_at IS NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_chat_messages_search;
