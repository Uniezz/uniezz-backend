-- +goose Up
CREATE TABLE chat_conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind TEXT NOT NULL CHECK (kind IN ('direct', 'group')),
    direct_user_low UUID REFERENCES users(id) ON DELETE CASCADE,
    direct_user_high UUID REFERENCES users(id) ON DELETE CASCADE,
    name TEXT CHECK (char_length(name) BETWEEN 1 AND 80),
    last_seq BIGINT NOT NULL DEFAULT 0,
    change_seq BIGINT NOT NULL DEFAULT 0,
    last_message_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_chat_conversations_direct_pair UNIQUE (direct_user_low, direct_user_high),
    CONSTRAINT chat_conversations_shape_check CHECK (
        (kind = 'direct' AND direct_user_low < direct_user_high AND name IS NULL)
        OR (kind = 'group' AND direct_user_low IS NULL AND direct_user_high IS NULL AND name IS NOT NULL)
    )
);

CREATE INDEX idx_chat_conversations_direct_user_high
    ON chat_conversations (direct_user_high) WHERE kind = 'direct';

CREATE TRIGGER set_chat_conversations_updated_at
BEFORE UPDATE ON chat_conversations
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE chat_members (
    conversation_id UUID NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'member')),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_read_seq BIGINT NOT NULL DEFAULT 0,
    unread_count INT NOT NULL DEFAULT 0 CHECK (unread_count >= 0),
    PRIMARY KEY (conversation_id, user_id)
);

CREATE INDEX idx_chat_members_user_id ON chat_members (user_id, conversation_id);
CREATE INDEX idx_chat_members_joined_at ON chat_members (conversation_id, joined_at, user_id);
CREATE UNIQUE INDEX uq_chat_members_owner ON chat_members (conversation_id) WHERE role = 'owner';

CREATE TABLE chat_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
    seq BIGINT NOT NULL,
    change_seq BIGINT NOT NULL,
    sender_id UUID REFERENCES users(id) ON DELETE SET NULL,
    kind TEXT NOT NULL CHECK (kind IN ('text', 'system')),
    body TEXT CHECK (char_length(body) <= 4000),
    payload JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(payload) = 'object'),
    client_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uq_chat_messages_seq UNIQUE (conversation_id, seq),
    CONSTRAINT uq_chat_messages_change_seq UNIQUE (conversation_id, change_seq)
);

CREATE UNIQUE INDEX uq_chat_messages_client ON chat_messages (sender_id, client_id) WHERE client_id IS NOT NULL;
CREATE INDEX idx_chat_messages_sender_id ON chat_messages (sender_id);

CREATE TABLE chat_message_media (
    message_id UUID NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    position SMALLINT NOT NULL CHECK (position BETWEEN 0 AND 4),
    media_id UUID NOT NULL UNIQUE REFERENCES media(id),
    PRIMARY KEY (message_id, position)
);

-- +goose Down
DROP TABLE chat_message_media;
DROP TABLE chat_messages;
DROP TABLE chat_members;
DROP TABLE chat_conversations;
