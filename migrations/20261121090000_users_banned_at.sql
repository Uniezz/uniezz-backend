-- +goose Up
ALTER TABLE users ADD COLUMN banned_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE users DROP COLUMN banned_at;
