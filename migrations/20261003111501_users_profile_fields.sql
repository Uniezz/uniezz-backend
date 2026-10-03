-- +goose Up
ALTER TABLE users
    ADD COLUMN first_name VARCHAR(100),
    ADD COLUMN last_name VARCHAR(100),
    ADD COLUMN last_login_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE users
    DROP COLUMN last_login_at,
    DROP COLUMN last_name,
    DROP COLUMN first_name;
