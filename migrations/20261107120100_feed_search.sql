-- +goose Up
CREATE EXTENSION IF NOT EXISTS unaccent WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;

-- The fixed unaccent dictionary makes this indexable. Reindex if its rules change.
-- +goose StatementBegin
CREATE FUNCTION public.feed_search_text(TEXT) RETURNS TEXT
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
AS $$ SELECT lower(public.unaccent('public.unaccent'::regdictionary, $1)) $$;
-- +goose StatementEnd

CREATE INDEX idx_feed_posts_search ON feed_posts USING gin
    (public.feed_search_text(title || ' ' || content || ' ' || university_id) public.gin_trgm_ops);
CREATE INDEX idx_users_feed_search_name ON users USING gin
    (public.feed_search_text(COALESCE(first_name, '') || ' ' || COALESCE(last_name, '')) public.gin_trgm_ops);
CREATE INDEX idx_feed_post_likes_created_at_post_id ON feed_post_likes (created_at, post_id);

-- +goose Down
DROP INDEX idx_feed_post_likes_created_at_post_id;
DROP INDEX idx_users_feed_search_name;
DROP INDEX idx_feed_posts_search;
DROP FUNCTION public.feed_search_text(TEXT);
