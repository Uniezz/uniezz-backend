-- +goose Up
CREATE TABLE feed_post_saves (
    post_id UUID NOT NULL REFERENCES feed_posts(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (post_id, user_id)
);

CREATE INDEX idx_feed_post_saves_user_created
    ON feed_post_saves (user_id, created_at DESC, post_id DESC);

DROP INDEX idx_feed_post_likes_user_id;
CREATE INDEX idx_feed_post_likes_user_created
    ON feed_post_likes (user_id, created_at DESC, post_id DESC);

-- +goose Down
DROP INDEX idx_feed_post_likes_user_created;
CREATE INDEX idx_feed_post_likes_user_id ON feed_post_likes (user_id);

DROP TABLE feed_post_saves;
