-- +goose Up
CREATE INDEX idx_reports_pending_created_at_id ON reports (created_at DESC, id DESC) WHERE status = 'pending';
DROP INDEX idx_reports_pending_created_at;

-- +goose Down
CREATE INDEX idx_reports_pending_created_at ON reports (created_at) WHERE status = 'pending';
DROP INDEX idx_reports_pending_created_at_id;
