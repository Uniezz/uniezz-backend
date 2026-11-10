-- +goose Up
CREATE TABLE faq_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    question VARCHAR(200) NOT NULL CHECK (char_length(question) BETWEEN 3 AND 200),
    answer TEXT NOT NULL CHECK (char_length(answer) BETWEEN 1 AND 2000),
    status TEXT NOT NULL CHECK (status IN ('proposed', 'published', 'rejected')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_faq_entries_published ON faq_entries (created_at DESC, id DESC) WHERE status = 'published';
CREATE INDEX idx_faq_entries_author ON faq_entries (author_id, created_at DESC, id DESC);

CREATE TRIGGER set_faq_entries_updated_at
BEFORE UPDATE ON faq_entries
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TABLE faq_entries;
