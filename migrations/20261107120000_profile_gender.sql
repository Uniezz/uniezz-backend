-- +goose Up
ALTER TABLE users
    ADD COLUMN gender TEXT CHECK (gender IN ('male', 'female'));

-- +goose Down
ALTER TABLE users DROP COLUMN gender;
