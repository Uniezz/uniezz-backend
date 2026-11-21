-- +goose Up
CREATE VIEW banned_users AS
SELECT id
FROM users
WHERE banned_at IS NOT NULL;

-- +goose Down
DROP VIEW banned_users;
