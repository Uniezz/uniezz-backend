-- +goose Up
ALTER TABLE users ADD COLUMN onboarded_at TIMESTAMPTZ;

-- Students who already have a name keep their place in the student list.
UPDATE users SET onboarded_at = created_at
WHERE first_name IS NOT NULL AND last_name IS NOT NULL;

-- +goose Down
ALTER TABLE users DROP COLUMN onboarded_at;
