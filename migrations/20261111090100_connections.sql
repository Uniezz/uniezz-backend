-- +goose Up
CREATE TABLE connection_profiles (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    birth_date DATE,
    open_to TEXT[] NOT NULL DEFAULT '{}' CHECK (open_to <@ ARRAY['friends', 'studyBuddy', 'dating']::text[]),
    intent TEXT CHECK (intent IS NULL OR char_length(intent) BETWEEN 1 AND 200),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_connection_profiles_open_to ON connection_profiles USING gin (open_to);

CREATE TRIGGER set_connection_profiles_updated_at
BEFORE UPDATE ON connection_profiles
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE discovery_candidates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    viewer_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (viewer_id, user_id)
);

CREATE INDEX idx_discovery_candidates_user_id ON discovery_candidates (user_id);

CREATE TABLE connection_actions (
    viewer_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mode TEXT NOT NULL CHECK (mode IN ('friends', 'studyBuddy', 'dating')),
    action TEXT NOT NULL CHECK (action IN ('like', 'pass')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (viewer_id, target_id, mode, action)
);

CREATE INDEX idx_connection_actions_target_id ON connection_actions (target_id);

CREATE TABLE hidden_candidates (
    viewer_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (viewer_id, target_id)
);

CREATE INDEX idx_hidden_candidates_target_id ON hidden_candidates (target_id);

CREATE TABLE connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_low UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_high UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mode TEXT NOT NULL CHECK (mode IN ('friends', 'studyBuddy', 'dating')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_low, user_high),
    CHECK (user_low < user_high)
);

CREATE INDEX idx_connections_user_high ON connections (user_high);

CREATE TABLE blocks (
    blocker_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    blocked_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (blocker_id, blocked_id),
    CHECK (blocker_id <> blocked_id)
);

CREATE INDEX idx_blocks_blocked_id ON blocks (blocked_id);

CREATE VIEW blocked_pairs AS
SELECT blocker_id AS user_id, blocked_id AS other_id FROM blocks
UNION ALL
SELECT blocked_id, blocker_id FROM blocks;

-- +goose Down
DROP VIEW blocked_pairs;
DROP TABLE blocks;
DROP TABLE connections;
DROP TABLE hidden_candidates;
DROP TABLE connection_actions;
DROP TABLE discovery_candidates;
DROP TABLE connection_profiles;
