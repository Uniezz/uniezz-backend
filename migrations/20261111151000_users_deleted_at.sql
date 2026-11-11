-- +goose Up
ALTER TABLE users ADD COLUMN deleted_at TIMESTAMPTZ;

CREATE INDEX idx_users_deleted_at ON users (deleted_at) WHERE deleted_at IS NOT NULL;

ALTER TABLE users DROP CONSTRAINT check_identity_present;
ALTER TABLE users ADD CONSTRAINT check_identity_present CHECK (
    email IS NOT NULL OR usos_user_id IS NOT NULL OR deleted_at IS NOT NULL
);

-- +goose Down
DELETE FROM users WHERE email IS NULL AND usos_user_id IS NULL;

ALTER TABLE users DROP CONSTRAINT check_identity_present;
ALTER TABLE users ADD CONSTRAINT check_identity_present CHECK (
    email IS NOT NULL OR usos_user_id IS NOT NULL
);

DROP INDEX idx_users_deleted_at;
ALTER TABLE users DROP COLUMN deleted_at;
