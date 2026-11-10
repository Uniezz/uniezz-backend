-- +goose Up
ALTER TABLE media DROP CONSTRAINT media_kind_check;
ALTER TABLE media ADD CONSTRAINT media_kind_check CHECK (kind IN ('avatar', 'banner', 'feed', 'chat'));

ALTER TABLE users
    ADD COLUMN banner_media_id UUID REFERENCES media(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE users
    DROP COLUMN banner_media_id;

DELETE FROM media WHERE kind = 'banner';
ALTER TABLE media DROP CONSTRAINT media_kind_check;
ALTER TABLE media ADD CONSTRAINT media_kind_check CHECK (kind IN ('avatar', 'feed', 'chat'));
