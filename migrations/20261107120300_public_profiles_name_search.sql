-- +goose Up
ALTER TABLE users ADD COLUMN name_search TEXT NOT NULL GENERATED ALWAYS AS
    (public.search_text(COALESCE(first_name, '') || ' ' || COALESCE(last_name, ''))) STORED;
CREATE INDEX idx_users_name_search ON users USING gin (name_search public.gin_trgm_ops);

CREATE OR REPLACE VIEW public_profiles AS
SELECT id, first_name, last_name, university_id, avatar_media_id, name_search
FROM users;

-- +goose Down
DROP VIEW public_profiles;
CREATE VIEW public_profiles AS
SELECT id, first_name, last_name, university_id, avatar_media_id
FROM users;

ALTER TABLE users DROP COLUMN name_search;
