-- +goose Up
CREATE INDEX idx_faq_entries_proposed ON faq_entries (created_at DESC, id DESC) WHERE status = 'proposed';

-- +goose Down
DROP INDEX idx_faq_entries_proposed;
