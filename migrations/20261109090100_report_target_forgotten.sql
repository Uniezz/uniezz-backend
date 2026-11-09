-- +goose Up
ALTER TABLE reports ADD COLUMN target_forgotten_at TIMESTAMPTZ;

CREATE INDEX idx_reports_target_author_id ON reports (target_author_id) WHERE target_author_id IS NOT NULL;
CREATE INDEX idx_reports_target_forgotten_at ON reports (target_forgotten_at) WHERE target_forgotten_at IS NOT NULL;

-- +goose Down
DROP INDEX idx_reports_target_forgotten_at;
DROP INDEX idx_reports_target_author_id;

ALTER TABLE reports DROP COLUMN target_forgotten_at;