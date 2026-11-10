-- +goose Up
ALTER TABLE users
    ADD COLUMN banner_media_id UUID REFERENCES media(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE users
    DROP COLUMN banner_media_id;
