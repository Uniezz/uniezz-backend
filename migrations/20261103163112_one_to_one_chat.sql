-- +goose Up
CREATE TABLE chat_threads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_low_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_high_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chat_threads_distinct_users CHECK (user_low_id < user_high_id),
    CONSTRAINT chat_threads_unique_pair UNIQUE (user_low_id, user_high_id)
);

CREATE INDEX idx_chat_threads_user_high_id ON chat_threads (user_high_id);

CREATE TABLE chat_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id UUID NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    sender_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL CHECK (char_length(btrim(content)) > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chat_messages_thread_created_at_id
    ON chat_messages (thread_id, created_at, id);

-- +goose Down
DROP TABLE chat_messages;
DROP TABLE chat_threads;