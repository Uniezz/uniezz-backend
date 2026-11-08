-- +goose Up
CREATE TABLE feed_post_weekly_likes (
    week DATE NOT NULL,
    post_id UUID NOT NULL REFERENCES feed_posts(id) ON DELETE CASCADE,
    post_created_at TIMESTAMPTZ NOT NULL,
    likes INT NOT NULL CHECK (likes >= 0),
    PRIMARY KEY (week, post_id)
);

CREATE INDEX idx_feed_post_weekly_likes_rank
    ON feed_post_weekly_likes (week, likes DESC, post_created_at DESC, post_id DESC);

INSERT INTO feed_post_weekly_likes (week, post_id, post_created_at, likes)
SELECT date_trunc('week', l.created_at AT TIME ZONE 'Europe/Warsaw')::date, p.id, p.created_at, count(*)
FROM feed_post_likes l
JOIN feed_posts p ON p.id = l.post_id
WHERE l.created_at >= date_trunc('week', NOW() AT TIME ZONE 'Europe/Warsaw') AT TIME ZONE 'Europe/Warsaw'
GROUP BY 1, p.id, p.created_at;

-- +goose Down
DROP TABLE feed_post_weekly_likes;
