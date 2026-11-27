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

CREATE TABLE profile_follow_counts (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    followers_count INT NOT NULL DEFAULT 0 CHECK (followers_count >= 0),
    following_count INT NOT NULL DEFAULT 0 CHECK (following_count >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER set_profile_follow_counts_updated_at
    BEFORE UPDATE ON profile_follow_counts
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Follow counts subtract follows of banned students; this keeps that lookup small.
CREATE INDEX idx_users_banned_at ON users (banned_at) WHERE banned_at IS NOT NULL;

-- +goose Down
DROP INDEX idx_users_banned_at;
DROP TRIGGER set_profile_follow_counts_updated_at ON profile_follow_counts;
DROP TABLE profile_follow_counts;
DROP TABLE profile_follows;
