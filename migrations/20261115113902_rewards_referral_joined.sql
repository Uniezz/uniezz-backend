-- +goose Up
ALTER TABLE coin_grants DROP CONSTRAINT coin_grants_reason_check;
ALTER TABLE coin_grants ADD CONSTRAINT coin_grants_reason_check
    CHECK (reason IN ('welcome', 'like_given', 'like_received', 'comment', 'profile_complete', 'birth_date', 'referral', 'referral_joined', 'daily_quests', 'faq_approved'));

-- +goose Down
WITH removed AS (
    DELETE FROM coin_grants
    WHERE reason = 'referral_joined'
    RETURNING user_id, amount
)
UPDATE coin_balances b
SET coins = GREATEST(b.coins - r.amount, 0)
FROM removed r
WHERE b.user_id = r.user_id;
ALTER TABLE coin_grants DROP CONSTRAINT coin_grants_reason_check;
ALTER TABLE coin_grants ADD CONSTRAINT coin_grants_reason_check
    CHECK (reason IN ('welcome', 'like_given', 'like_received', 'comment', 'profile_complete', 'birth_date', 'referral', 'daily_quests', 'faq_approved'));
