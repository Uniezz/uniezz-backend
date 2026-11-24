-- +goose Up
CREATE VIEW user_languages AS
SELECT id, language
FROM users
WHERE deleted_at IS NULL;

-- +goose Down
DROP VIEW user_languages;
