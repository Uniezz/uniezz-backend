-- +goose Up
DELETE FROM chat_conversations
WHERE kind = 'direct' AND origin = 'request' AND status = 'pending' AND last_seq = 0;

-- +goose Down
-- the deleted requests held no messages; there is nothing to restore
SELECT 1;
