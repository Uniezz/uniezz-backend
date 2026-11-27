-- +goose Up
CREATE TABLE chat_poll_votes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    option_id SMALLINT NOT NULL CHECK (option_id BETWEEN 1 AND 10),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uq_chat_poll_votes_vote ON chat_poll_votes (message_id, user_id, option_id);
CREATE INDEX idx_chat_poll_votes_user_id ON chat_poll_votes (user_id);

-- +goose Down
DROP TABLE chat_poll_votes;
