-- +goose Up
ALTER TABLE users
    ADD COLUMN bio VARCHAR(500),
    ADD COLUMN interests TEXT[] NOT NULL DEFAULT '{}' CHECK (cardinality(interests) <= 10),
    ADD COLUMN privacy JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(privacy) = 'object');

CREATE INDEX idx_users_created_at_id ON users (created_at DESC, id DESC);

-- +goose Down
DROP INDEX idx_users_created_at_id;
ALTER TABLE users
    DROP COLUMN privacy,
    DROP COLUMN interests,
    DROP COLUMN bio;
