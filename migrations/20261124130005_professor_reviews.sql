-- +goose Up
CREATE TABLE professor_reviews (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    professor_id UUID NOT NULL REFERENCES professors(id) ON DELETE CASCADE,
    author_id UUID NOT NULL REFERENCES users(id),
    content TEXT NOT NULL CHECK (char_length(content) BETWEEN 1 AND 1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER set_professor_reviews_updated_at
BEFORE UPDATE ON professor_reviews
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

CREATE UNIQUE INDEX uq_professor_reviews_author ON professor_reviews (professor_id, author_id);
CREATE INDEX idx_professor_reviews_professor_id_created_at ON professor_reviews (professor_id, created_at DESC, id DESC);
CREATE INDEX idx_professor_reviews_author_id ON professor_reviews (author_id);

-- +goose Down
DROP TABLE professor_reviews;
