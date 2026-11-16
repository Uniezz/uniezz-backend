-- +goose Up
CREATE TABLE coin_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason TEXT NOT NULL CHECK (reason IN ('welcome', 'like_given', 'like_received', 'comment', 'profile_complete', 'birth_date', 'referral')),
    ref_id UUID,
    actor_id UUID,
    amount INT NOT NULL CHECK (amount > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One grant per action; NULLS NOT DISTINCT makes once-per-account reasons (no ref, no actor) unique too.
CREATE UNIQUE INDEX uq_coin_grants_action
    ON coin_grants (user_id, reason, ref_id, actor_id) NULLS NOT DISTINCT;

CREATE TABLE coin_balances (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    coins INT NOT NULL CHECK (coins >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER set_coin_balances_updated_at
BEFORE UPDATE ON coin_balances
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

INSERT INTO coin_grants (user_id, reason, amount)
SELECT id, 'welcome', 100 FROM users;

INSERT INTO coin_balances (user_id, coins)
SELECT user_id, SUM(amount) FROM coin_grants GROUP BY user_id;

-- +goose Down
DROP TABLE coin_balances;
DROP TABLE coin_grants;
