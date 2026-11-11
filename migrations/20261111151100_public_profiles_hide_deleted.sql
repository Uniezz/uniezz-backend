-- +goose Up
CREATE OR REPLACE VIEW public_profiles AS
SELECT id, first_name, last_name, university_id, avatar_media_id, name_search
FROM users
WHERE deleted_at IS NULL;

-- +goose Down
CREATE OR REPLACE VIEW public_profiles AS
SELECT id, first_name, last_name, university_id, avatar_media_id, name_search
FROM users;
