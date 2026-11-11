-- +goose Up
ALTER TABLE media DROP CONSTRAINT media_owner_user_id_fkey;
ALTER TABLE media ADD CONSTRAINT media_owner_user_id_fkey
    FOREIGN KEY (owner_user_id) REFERENCES users(id);

-- +goose Down
ALTER TABLE media DROP CONSTRAINT media_owner_user_id_fkey;
ALTER TABLE media ADD CONSTRAINT media_owner_user_id_fkey
    FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE CASCADE;
