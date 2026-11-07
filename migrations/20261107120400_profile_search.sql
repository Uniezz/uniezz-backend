-- +goose Up
-- array_to_string is only STABLE, but it is immutable for text[], so this is safe to index.
-- +goose StatementBegin
CREATE FUNCTION public.search_text(TEXT[]) RETURNS TEXT
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
AS $$ SELECT public.search_text(array_to_string($1, ' ')) $$;
-- +goose StatementEnd

CREATE INDEX idx_users_interests_search ON users USING gin (public.search_text(interests) public.gin_trgm_ops);
CREATE INDEX idx_users_university_created_at_id ON users (university_id, created_at DESC, id DESC);
CREATE INDEX idx_student_programmes_programme_search ON student_programmes
    USING gin (public.search_text(programme_name) public.gin_trgm_ops);
CREATE INDEX idx_student_programmes_faculty_search ON student_programmes
    USING gin (public.search_text(faculty_name) public.gin_trgm_ops);

-- +goose Down
DROP INDEX idx_student_programmes_faculty_search;
DROP INDEX idx_student_programmes_programme_search;
DROP INDEX idx_users_university_created_at_id;
DROP INDEX idx_users_interests_search;
DROP FUNCTION public.search_text(TEXT[]);
