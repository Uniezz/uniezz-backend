-- +goose Up
ALTER TABLE jobs ADD COLUMN unique_key TEXT;
CREATE UNIQUE INDEX uq_jobs_unique_key ON jobs (unique_key) WHERE unique_key IS NOT NULL;

CREATE INDEX idx_media_deleting ON media (created_at) WHERE status = 'deleting';

-- +goose Down
DROP INDEX idx_media_deleting;
DROP INDEX uq_jobs_unique_key;
ALTER TABLE jobs DROP COLUMN unique_key;
