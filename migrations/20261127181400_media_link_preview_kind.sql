-- +goose Up
SET LOCAL lock_timeout = '3s';

ALTER TABLE media DROP CONSTRAINT media_kind_check;
ALTER TABLE media ADD CONSTRAINT media_kind_check
    CHECK (kind IN ('avatar', 'banner', 'feed', 'chat', 'exam', 'link_preview')) NOT VALID;
ALTER TABLE media VALIDATE CONSTRAINT media_kind_check;

-- +goose Down
SET LOCAL lock_timeout = '3s';

DELETE FROM media WHERE kind = 'link_preview';
ALTER TABLE media DROP CONSTRAINT media_kind_check;
ALTER TABLE media ADD CONSTRAINT media_kind_check
    CHECK (kind IN ('avatar', 'banner', 'feed', 'chat', 'exam')) NOT VALID;
ALTER TABLE media VALIDATE CONSTRAINT media_kind_check;
