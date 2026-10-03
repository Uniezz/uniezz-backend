-- +goose Up

-- +goose StatementBegin
CREATE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';
-- +goose StatementEnd

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    university_id VARCHAR(32) NOT NULL,
    email VARCHAR(255),
    usos_user_id VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT check_identity_present CHECK (
        email IS NOT NULL OR usos_user_id IS NOT NULL
    )
);

CREATE TRIGGER set_users_updated_at
BEFORE UPDATE ON users
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

CREATE UNIQUE INDEX idx_users_university_email 
ON users (university_id, LOWER(email)) 
WHERE email IS NOT NULL;

CREATE UNIQUE INDEX idx_users_university_usos 
ON users (university_id, usos_user_id) 
WHERE usos_user_id IS NOT NULL;


CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_sessions_token_hash UNIQUE (token_hash)
);

CREATE INDEX idx_sessions_user_id 
ON sessions (user_id);


-- +goose Down
DROP TABLE sessions;
DROP TRIGGER set_users_updated_at ON users;
DROP TABLE users;
DROP FUNCTION update_updated_at_column;
