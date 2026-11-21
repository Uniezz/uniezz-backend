-- +goose Up
CREATE VIEW staff_roles AS
SELECT user_id, role, created_at
FROM user_roles;

-- +goose Down
DROP VIEW staff_roles;
