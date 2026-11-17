-- +goose Up
ALTER TABLE users
    ADD COLUMN invited_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD CONSTRAINT check_users_invited_by_not_self CHECK (invited_by <> id);

CREATE INDEX idx_users_invited_by ON users (invited_by);

-- Students who joined through an invitation before this column existed are known only from their coin grant.
ALTER TABLE users DISABLE TRIGGER set_users_updated_at;
UPDATE users u
SET invited_by = g.actor_id
FROM coin_grants g
WHERE g.user_id = u.id
    AND g.reason = 'referral_joined'
    AND g.actor_id <> u.id
    AND EXISTS (SELECT 1 FROM users i WHERE i.id = g.actor_id);
ALTER TABLE users ENABLE TRIGGER set_users_updated_at;

-- +goose Down
DROP INDEX idx_users_invited_by;
ALTER TABLE users
    DROP CONSTRAINT check_users_invited_by_not_self,
    DROP COLUMN invited_by;
