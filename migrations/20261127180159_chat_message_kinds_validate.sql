-- +goose Up
ALTER TABLE chat_messages VALIDATE CONSTRAINT chat_messages_kind_check;

-- +goose Down
SELECT 1;
