-- +goose Up

CREATE TABLE oauth_states (
    request_token  TEXT PRIMARY KEY,
    request_secret TEXT NOT NULL,
    university_id  VARCHAR(32) NOT NULL,
    platform       VARCHAR(16) NOT NULL CHECK (platform IN ('web', 'mobile')),
    expires_at     TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '10 minutes'
);


-- +goose Down
DROP TABLE oauth_states;
