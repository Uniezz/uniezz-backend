-- +goose Up
CREATE TABLE chat_event_answers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    answer TEXT NOT NULL CHECK (answer IN ('going', 'maybe', 'no')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uq_chat_event_answers_user ON chat_event_answers (message_id, user_id);
CREATE INDEX idx_chat_event_answers_user_id ON chat_event_answers (user_id);

CREATE TRIGGER set_chat_event_answers_updated_at
    BEFORE UPDATE ON chat_event_answers
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TRIGGER set_chat_event_answers_updated_at ON chat_event_answers;
DROP TABLE chat_event_answers;
