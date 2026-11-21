-- +goose Up
CREATE INDEX idx_user_roles_role_created_at_user_id ON user_roles (role, created_at DESC, user_id DESC);

-- +goose Down
DROP INDEX idx_user_roles_role_created_at_user_id;
