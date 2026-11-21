-- +goose Up
CREATE TABLE moderation_actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    type TEXT NOT NULL CHECK (type IN (
        'review_report', 'dismiss_report', 'remove_post', 'remove_comment', 'remove_file', 'remove_course',
        'grant_moderator', 'revoke_moderator', 'ban', 'unban'
    )),
    target_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_moderation_actions_created_at_id ON moderation_actions (created_at DESC, id DESC);
CREATE INDEX idx_moderation_actions_actor_id ON moderation_actions (actor_id);

-- +goose Down
DROP TABLE moderation_actions;
