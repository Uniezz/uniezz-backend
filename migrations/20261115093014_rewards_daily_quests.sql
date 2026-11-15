-- +goose Up
ALTER TABLE coin_grants DROP CONSTRAINT coin_grants_reason_check;
ALTER TABLE coin_grants ADD CONSTRAINT coin_grants_reason_check
    CHECK (reason IN ('welcome', 'like_given', 'like_received', 'comment', 'profile_complete', 'birth_date', 'referral', 'daily_quests'));

CREATE INDEX idx_coin_grants_user_created ON coin_grants (user_id, created_at DESC, id DESC);

CREATE TABLE daily_quests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day DATE NOT NULL,
    likes INT NOT NULL DEFAULT 0 CHECK (likes >= 0),
    replied BOOLEAN NOT NULL DEFAULT FALSE,
    posted BOOLEAN NOT NULL DEFAULT FALSE,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uq_daily_quests_user_day ON daily_quests (user_id, day);
CREATE INDEX idx_daily_quests_day ON daily_quests (day);

CREATE TRIGGER set_daily_quests_updated_at
BEFORE UPDATE ON daily_quests
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TABLE daily_quests;

DROP INDEX idx_coin_grants_user_created;

WITH removed AS (
    DELETE FROM coin_grants
    WHERE reason = 'daily_quests'
    RETURNING user_id, amount
)
UPDATE coin_balances b
SET coins = GREATEST(b.coins - r.amount, 0)
FROM removed r
WHERE b.user_id = r.user_id;

ALTER TABLE coin_grants DROP CONSTRAINT coin_grants_reason_check;
ALTER TABLE coin_grants ADD CONSTRAINT coin_grants_reason_check
    CHECK (reason IN ('welcome', 'like_given', 'like_received', 'comment', 'profile_complete', 'birth_date', 'referral'));
