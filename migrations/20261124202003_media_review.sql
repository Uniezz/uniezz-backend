-- +goose Up
ALTER TABLE media DROP CONSTRAINT media_status_check;
ALTER TABLE media ADD CONSTRAINT media_status_check CHECK (status IN ('pending', 'ready', 'deleting', 'removed'));
ALTER TABLE media ADD COLUMN reviewed_at TIMESTAMPTZ;

CREATE INDEX idx_media_unreviewed_created_at ON media (created_at, id) WHERE reviewed_at IS NULL;

-- +goose Down
DROP INDEX idx_media_unreviewed_created_at;
ALTER TABLE media DROP COLUMN reviewed_at;
UPDATE media SET status = 'deleting' WHERE status = 'removed';
ALTER TABLE media DROP CONSTRAINT media_status_check;
ALTER TABLE media ADD CONSTRAINT media_status_check CHECK (status IN ('pending', 'ready', 'deleting'));
