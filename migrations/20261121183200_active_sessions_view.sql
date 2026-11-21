-- +goose Up
CREATE VIEW active_sessions AS
SELECT s.id, s.user_id
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.expires_at > NOW() AND u.deleted_at IS NULL AND u.banned_at IS NULL;

-- +goose Down
DROP VIEW active_sessions;
