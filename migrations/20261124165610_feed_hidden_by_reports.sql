-- +goose Up
ALTER TABLE feed_posts
    ADD COLUMN hidden_at TIMESTAMPTZ,
    ADD COLUMN hide_cleared_at TIMESTAMPTZ;

ALTER TABLE feed_comments
    ADD COLUMN hidden_at TIMESTAMPTZ,
    ADD COLUMN hide_cleared_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE feed_comments
    DROP COLUMN hide_cleared_at,
    DROP COLUMN hidden_at;

ALTER TABLE feed_posts
    DROP COLUMN hide_cleared_at,
    DROP COLUMN hidden_at;
