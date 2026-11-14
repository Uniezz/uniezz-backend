-- +goose Up
CREATE INDEX idx_courses_name_search ON courses USING gin (public.search_text(name) public.gin_trgm_ops);
CREATE INDEX idx_professors_name_search ON professors USING gin (public.search_text(name) public.gin_trgm_ops) WHERE active;

-- +goose Down
DROP INDEX idx_professors_name_search;
DROP INDEX idx_courses_name_search;
