-- +goose Up
ALTER TABLE media DROP CONSTRAINT media_kind_check;
ALTER TABLE media ADD CONSTRAINT media_kind_check
    CHECK (kind IN ('avatar', 'banner', 'feed', 'chat', 'exam'));

ALTER TABLE media DROP CONSTRAINT media_content_type_check;
ALTER TABLE media ADD CONSTRAINT media_content_type_check
    CHECK (content_type IN ('image/jpeg', 'image/png', 'image/webp', 'application/pdf'));

-- +goose Down
DELETE FROM media WHERE kind = 'exam' OR content_type = 'application/pdf';

ALTER TABLE media DROP CONSTRAINT media_content_type_check;
ALTER TABLE media ADD CONSTRAINT media_content_type_check
    CHECK (content_type IN ('image/jpeg', 'image/png', 'image/webp'));

ALTER TABLE media DROP CONSTRAINT media_kind_check;
ALTER TABLE media ADD CONSTRAINT media_kind_check
    CHECK (kind IN ('avatar', 'banner', 'feed', 'chat'));
