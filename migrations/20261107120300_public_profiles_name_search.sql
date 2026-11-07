-- +goose Up
CREATE INDEX idx_users_name_search ON users USING gin
    (public.search_text(COALESCE(first_name, '') || ' ' || COALESCE(last_name, '')) public.gin_trgm_ops);

-- +goose Down
DROP INDEX idx_users_name_search;
