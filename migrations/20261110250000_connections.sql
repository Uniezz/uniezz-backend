-- +goose Up
ALTER TABLE users
    ADD COLUMN birth_date DATE,
    ADD COLUMN open_to TEXT[] NOT NULL DEFAULT '{}' CHECK (open_to <@ ARRAY['friends', 'studyBuddy', 'dating']::text[]),
    ADD COLUMN intent TEXT CHECK (intent IS NULL OR char_length(intent) BETWEEN 1 AND 200);

CREATE TABLE discovery_candidates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    viewer_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (viewer_id, user_id)
);

CREATE TABLE connection_actions (
    viewer_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mode TEXT NOT NULL CHECK (mode IN ('friends', 'studyBuddy', 'dating')),
    action TEXT NOT NULL CHECK (action IN ('like', 'pass', 'hide')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (viewer_id, target_id, mode, action)
);

CREATE TABLE connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_low UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_high UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mode TEXT NOT NULL CHECK (mode IN ('friends', 'studyBuddy', 'dating')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_low, user_high),
    CHECK (user_low < user_high)
);

CREATE TABLE blocks (
    blocker_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    blocked_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (blocker_id, blocked_id),
    CHECK (blocker_id <> blocked_id)
);

ALTER TABLE reports DROP CONSTRAINT reports_target_type_check;
ALTER TABLE reports ADD CONSTRAINT reports_target_type_check
    CHECK (target_type IN ('post', 'comment', 'user', 'candidate'));

-- +goose Down
ALTER TABLE reports DROP CONSTRAINT reports_target_type_check;
ALTER TABLE reports ADD CONSTRAINT reports_target_type_check
    CHECK (target_type IN ('post', 'comment', 'user'));

DROP TABLE blocks;
DROP TABLE connections;
DROP TABLE connection_actions;
DROP TABLE discovery_candidates;

ALTER TABLE users
    DROP COLUMN birth_date,
    DROP COLUMN open_to,
    DROP COLUMN intent;
