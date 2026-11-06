-- +goose Up
ALTER TABLE users ADD COLUMN deleted_at TIMESTAMPTZ;

CREATE INDEX idx_users_deleted_at ON users (deleted_at) WHERE deleted_at IS NOT NULL;

ALTER TABLE users DROP CONSTRAINT check_identity_present;
ALTER TABLE users ADD CONSTRAINT check_identity_present CHECK (
    email IS NOT NULL OR usos_user_id IS NOT NULL OR deleted_at IS NOT NULL
);

CREATE OR REPLACE VIEW public_profiles AS
SELECT id, first_name, last_name, university_id, avatar_media_id
FROM users
WHERE deleted_at IS NULL; 

ALTER TABLE media DROP CONSTRAINT media_owner_user_id_fkey;
ALTER TABLE media ADD CONSTRAINT media_owner_user_id_fkey
    FOREIGN KEY (owner_user_id) REFERENCES users(id);

-- +goose Down
ALTER TABLE media DROP CONSTRAINT media_owner_user_id_fkey;
ALTER TABLE media ADD CONSTRAINT media_owner_user_id_fkey
    FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE CASCADE;

CREATE OR REPLACE VIEW public_profiles AS
SELECT id, first_name, last_name, university_id, avatar_media_id
FROM users;

ALTER TABLE users DROP CONSTRAINT check_identity_present;
ALTER TABLE users ADD CONSTRAINT check_identity_present CHECK (
    email IS NOT NULL OR usos_user_id IS NOT NULL
);

DROP INDEX idx_users_deleted_at;
ALTER TABLE users DROP COLUMN deleted_at;