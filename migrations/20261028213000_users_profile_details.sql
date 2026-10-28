-- +goose Up
ALTER TABLE users
    ADD COLUMN major VARCHAR(255),
    ADD COLUMN current_year INT CHECK (current_year BETWEEN 1 AND 6),
    ADD COLUMN avatar VARCHAR(512);

-- +goose Down
ALTER TABLE users
    DROP COLUMN avatar,
    DROP COLUMN current_year,
    DROP COLUMN major;