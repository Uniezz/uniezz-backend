-- +goose Up
WITH granted AS (
    INSERT INTO coin_grants (user_id, reason, amount)
    SELECT id, 'profile_complete', 50
    FROM users
    WHERE first_name IS NOT NULL AND last_name IS NOT NULL AND gender IS NOT NULL
        AND avatar_media_id IS NOT NULL AND banner_media_id IS NOT NULL
        AND bio IS NOT NULL AND song_url IS NOT NULL AND cardinality(interests) > 0
    ON CONFLICT DO NOTHING
    RETURNING user_id, amount
)
INSERT INTO coin_balances (user_id, coins)
SELECT user_id, amount FROM granted
ON CONFLICT (user_id) DO UPDATE SET coins = coin_balances.coins + EXCLUDED.coins;

-- +goose Down
WITH removed AS (
    DELETE FROM coin_grants
    WHERE reason = 'profile_complete'
    RETURNING user_id, amount
)
UPDATE coin_balances b
SET coins = GREATEST(b.coins - r.amount, 0)
FROM removed r
WHERE b.user_id = r.user_id;
