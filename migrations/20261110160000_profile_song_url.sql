-- +goose Up
ALTER TABLE users
    ADD COLUMN song_url TEXT;

-- +goose Down
ALTER TABLE users DROP COLUMN song_url;
