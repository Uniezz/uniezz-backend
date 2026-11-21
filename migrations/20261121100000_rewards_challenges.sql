-- +goose Up
CREATE TABLE challenges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    action TEXT NOT NULL CHECK (action IN (
        'like_given', 'like_received', 'comment_posted', 'reply_posted', 'post_created', 'file_uploaded',
        'course_rated', 'professor_rated', 'connection_made', 'profile_completed', 'friend_invited', 'faq_approved'
    )),
    target INT NOT NULL CHECK (target BETWEEN 1 AND 1000),
    period TEXT NOT NULL CHECK (period IN ('day', 'week', 'once')),
    reward INT NOT NULL CHECK (reward BETWEEN 0 AND 1000),
    texts JSONB NOT NULL CHECK (jsonb_typeof(texts) = 'object' AND texts ? 'pl'),
    starts_at TIMESTAMPTZ,
    ends_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (starts_at IS NULL OR ends_at IS NULL OR ends_at > starts_at)
);

CREATE INDEX idx_challenges_action ON challenges (action) WHERE archived_at IS NULL;
CREATE INDEX idx_challenges_created_at_id ON challenges (created_at DESC, id DESC);

CREATE TRIGGER set_challenges_updated_at
BEFORE UPDATE ON challenges
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE challenge_progress (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    challenge_id UUID NOT NULL REFERENCES challenges(id) ON DELETE CASCADE,
    period_start DATE NOT NULL,
    target INT NOT NULL CHECK (target >= 1),
    progress INT NOT NULL CHECK (progress >= 0),
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uq_challenge_progress_user_challenge_period ON challenge_progress (user_id, challenge_id, period_start);
CREATE INDEX idx_challenge_progress_challenge_id ON challenge_progress (challenge_id);
CREATE INDEX idx_challenge_progress_period_start ON challenge_progress (period_start);

CREATE TRIGGER set_challenge_progress_updated_at
BEFORE UPDATE ON challenge_progress
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE coin_grants DROP CONSTRAINT coin_grants_reason_check;
ALTER TABLE coin_grants ADD CONSTRAINT coin_grants_reason_check
    CHECK (reason IN ('welcome', 'like_given', 'like_received', 'comment', 'profile_complete', 'birth_date', 'referral', 'referral_joined', 'daily_quests', 'faq_approved', 'challenge'));

-- +goose Down
WITH removed AS (
    DELETE FROM coin_grants
    WHERE reason = 'challenge'
    RETURNING user_id, amount
)
UPDATE coin_balances b
SET coins = GREATEST(b.coins - r.amount, 0)
FROM removed r
WHERE b.user_id = r.user_id;
ALTER TABLE coin_grants DROP CONSTRAINT coin_grants_reason_check;
ALTER TABLE coin_grants ADD CONSTRAINT coin_grants_reason_check
    CHECK (reason IN ('welcome', 'like_given', 'like_received', 'comment', 'profile_complete', 'birth_date', 'referral', 'referral_joined', 'daily_quests', 'faq_approved'));

DROP TABLE challenge_progress;
DROP TABLE challenges;
