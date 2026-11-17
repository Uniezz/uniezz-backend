-- +goose Up
ALTER TABLE users
    ADD COLUMN invite_code CHAR(7);

ALTER TABLE users DISABLE TRIGGER set_users_updated_at;
-- +goose StatementBegin
DO $$
DECLARE
    user_row RECORD;
    candidate CHAR(7);
BEGIN
    FOR user_row IN SELECT id FROM users LOOP
        LOOP
            candidate := UPPER(SUBSTRING(MD5(gen_random_uuid()::TEXT), 1, 7));
            EXIT WHEN NOT EXISTS (SELECT 1 FROM users WHERE invite_code = candidate);
        END LOOP;
        UPDATE users SET invite_code = candidate WHERE id = user_row.id;
    END LOOP;
END $$;
-- +goose StatementEnd
ALTER TABLE users ENABLE TRIGGER set_users_updated_at;

ALTER TABLE users
    ALTER COLUMN invite_code SET NOT NULL,
    ALTER COLUMN invite_code SET DEFAULT UPPER(SUBSTRING(MD5(gen_random_uuid()::TEXT), 1, 7)),
    ADD CONSTRAINT check_users_invite_code CHECK (invite_code ~ '^[A-Z0-9]{7}$');

CREATE UNIQUE INDEX uq_users_invite_code ON users (invite_code);

ALTER TABLE oauth_states ADD COLUMN referral_code CHAR(7);

-- +goose Down
ALTER TABLE oauth_states DROP COLUMN referral_code;
DROP INDEX uq_users_invite_code;
ALTER TABLE users DROP CONSTRAINT check_users_invite_code;
ALTER TABLE users DROP COLUMN invite_code;
