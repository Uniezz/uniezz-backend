-- +goose Up
SET LOCAL lock_timeout = '3s';

ALTER TABLE chat_messages DROP CONSTRAINT chat_messages_kind_check;
ALTER TABLE chat_messages ADD CONSTRAINT chat_messages_kind_check
    CHECK (kind IN ('text', 'system', 'poll', 'event', 'location', 'post')) NOT VALID;

-- +goose Down
SET LOCAL lock_timeout = '3s';

DELETE FROM chat_messages WHERE kind NOT IN ('text', 'system');
ALTER TABLE chat_messages DROP CONSTRAINT chat_messages_kind_check;
ALTER TABLE chat_messages ADD CONSTRAINT chat_messages_kind_check
    CHECK (kind IN ('text', 'system')) NOT VALID;
ALTER TABLE chat_messages VALIDATE CONSTRAINT chat_messages_kind_check;
