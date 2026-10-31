-- +goose Up
CREATE TABLE reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reporter_id UUID REFERENCES users(id) ON DELETE SET NULL,
    target_type TEXT NOT NULL CHECK (target_type IN ('post', 'comment', 'user')),
    target_id UUID NOT NULL,
    target_author_id UUID,
    reason TEXT NOT NULL CHECK (reason IN ('spam', 'harassment', 'inappropriate', 'hate_speech', 'other')),
    details VARCHAR(1000),
    snapshot JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(snapshot) = 'object'),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'reviewed', 'dismissed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_reports_reporter_target UNIQUE (reporter_id, target_type, target_id)
);

CREATE INDEX idx_reports_target ON reports (target_type, target_id);
CREATE INDEX idx_reports_pending_created_at ON reports (created_at) WHERE status = 'pending';

-- +goose Down
DROP TABLE reports;
