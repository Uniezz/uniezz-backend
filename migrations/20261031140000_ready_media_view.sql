-- +goose Up
CREATE VIEW ready_media AS
SELECT id, kind, object_key
FROM media
WHERE status = 'ready';

-- +goose Down
DROP VIEW ready_media;
