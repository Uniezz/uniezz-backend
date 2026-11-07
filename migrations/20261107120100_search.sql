-- +goose Up
CREATE EXTENSION IF NOT EXISTS unaccent WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;

-- The fixed unaccent dictionary makes this indexable. Reindex if its rules change.
-- +goose StatementBegin
CREATE FUNCTION public.search_text(TEXT) RETURNS TEXT
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
AS $$ SELECT lower(public.unaccent('public.unaccent'::regdictionary, $1)) $$;
-- +goose StatementEnd

CREATE INDEX idx_feed_posts_search ON feed_posts USING gin
    (public.search_text(title || ' ' || content) public.gin_trgm_ops);

-- +goose Down
DROP INDEX idx_feed_posts_search;
DROP FUNCTION public.search_text(TEXT);
