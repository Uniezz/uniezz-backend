-- +goose Up
CREATE VIEW user_emails AS
SELECT id, email
FROM users;

-- +goose Down
DROP VIEW user_emails;
