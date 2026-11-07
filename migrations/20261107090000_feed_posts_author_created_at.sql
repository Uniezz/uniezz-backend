-- +goose Up
CREATE INDEX idx_feed_posts_author_created_at_id ON feed_posts (author_id, created_at DESC, id DESC);
DROP INDEX idx_feed_posts_author_id;

-- +goose Down
CREATE INDEX idx_feed_posts_author_id ON feed_posts (author_id);
DROP INDEX idx_feed_posts_author_created_at_id;
