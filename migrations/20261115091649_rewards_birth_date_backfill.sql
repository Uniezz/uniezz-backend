-- +goose Up
WITH granted AS (
    INSERT INTO coin_grants (user_id, reason, amount)
    SELECT user_id, 'birth_date', 10
    FROM connection_profiles
    WHERE birth_date IS NOT NULL
    ON CONFLICT DO NOTHING
    RETURNING user_id, amount
)
INSERT INTO coin_balances (user_id, coins)
SELECT user_id, amount FROM granted
ON CONFLICT (user_id) DO UPDATE SET coins = coin_balances.coins + EXCLUDED.coins;

-- +goose Down
WITH removed AS (
    DELETE FROM coin_grants
    WHERE reason = 'birth_date'
    RETURNING user_id, amount
)
UPDATE coin_balances b
SET coins = GREATEST(b.coins - r.amount, 0)
FROM removed r
WHERE b.user_id = r.user_id;
