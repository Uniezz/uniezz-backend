-- +goose NO TRANSACTION
-- +goose Up
CREATE EXTENSION IF NOT EXISTS btree_gin WITH SCHEMA public;

-- conversation_id in the same GIN index lets a search read only the matches of one conversation.
CREATE INDEX CONCURRENTLY idx_chat_messages_conversation_id_search ON chat_messages
    USING gin (conversation_id, public.search_text(body) public.gin_trgm_ops)
    WHERE body IS NOT NULL AND deleted_at IS NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_chat_messages_conversation_id_search;
DROP EXTENSION IF EXISTS btree_gin;
