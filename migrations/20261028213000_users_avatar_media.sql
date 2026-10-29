-- +goose Up
ALTER TABLE media DROP CONSTRAINT media_status_check;
ALTER TABLE media ADD CONSTRAINT media_status_check CHECK (status IN ('pending', 'ready', 'deleting'));

ALTER TABLE users
    ADD COLUMN avatar_media_id UUID REFERENCES media(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE users
    DROP COLUMN avatar_media_id;

DELETE FROM media WHERE status = 'deleting';
ALTER TABLE media DROP CONSTRAINT media_status_check;
ALTER TABLE media ADD CONSTRAINT media_status_check CHECK (status IN ('pending', 'ready'));
