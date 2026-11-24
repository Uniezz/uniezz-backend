-- +goose Up
CREATE TABLE push_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token TEXT NOT NULL CHECK (char_length(token) <= 200),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_push_tokens_session UNIQUE (session_id),
    CONSTRAINT uq_push_tokens_token UNIQUE (token)
);

CREATE INDEX idx_push_tokens_user_id ON push_tokens (user_id);

CREATE TRIGGER set_push_tokens_updated_at
BEFORE UPDATE ON push_tokens
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE push_settings (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER set_push_settings_updated_at
BEFORE UPDATE ON push_settings
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE push_deliveries (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    collapse_id UUID NOT NULL,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, collapse_id)
);

CREATE INDEX idx_push_deliveries_sent_at ON push_deliveries (sent_at);

-- +goose Down
DROP TABLE push_deliveries;
DROP TABLE push_settings;
DROP TABLE push_tokens;
