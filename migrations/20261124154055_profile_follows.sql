-- +goose Up
CREATE TABLE profile_follows (
    follower_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    followed_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (follower_id, followed_id),
    CHECK (follower_id <> followed_id)
);

CREATE INDEX idx_profile_follows_followed_created_at
    ON profile_follows (followed_id, created_at DESC, follower_id DESC);
CREATE INDEX idx_profile_follows_follower_created_at
    ON profile_follows (follower_id, created_at DESC, followed_id DESC);

-- +goose Down
DROP TABLE profile_follows;
