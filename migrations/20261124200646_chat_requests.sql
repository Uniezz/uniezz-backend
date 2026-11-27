-- +goose Up
ALTER TABLE chat_conversations
    ADD COLUMN status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'declined')),
    ADD COLUMN origin TEXT CHECK (origin IN ('match', 'request')),
    ADD COLUMN started_by UUID REFERENCES users(id) ON DELETE CASCADE;

UPDATE chat_conversations SET status = 'approved';
UPDATE chat_conversations SET origin = 'match', started_by = direct_user_low WHERE kind = 'direct';

ALTER TABLE chat_conversations ADD CONSTRAINT chat_conversations_request_check CHECK (
    (kind = 'direct' AND origin IS NOT NULL AND started_by IS NOT NULL)
    OR (kind = 'group' AND origin IS NULL AND started_by IS NULL AND status = 'approved')
);

CREATE INDEX idx_chat_conversations_started_by ON chat_conversations (started_by);

CREATE TABLE chat_settings (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    who_can_message TEXT NOT NULL CHECK (who_can_message IN ('everyone', 'university', 'matches')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER set_chat_settings_updated_at
BEFORE UPDATE ON chat_settings
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TABLE chat_settings;
DROP INDEX idx_chat_conversations_started_by;
ALTER TABLE chat_conversations DROP CONSTRAINT chat_conversations_request_check;
ALTER TABLE chat_conversations
    DROP COLUMN started_by,
    DROP COLUMN origin,
    DROP COLUMN status;
