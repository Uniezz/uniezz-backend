-- +goose Up
CREATE VIEW public_profiles AS
SELECT id, first_name, last_name, university_id, avatar_media_id
FROM users;

-- +goose Down
DROP VIEW public_profiles;
