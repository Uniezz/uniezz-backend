-- +goose Up
ALTER TABLE users DROP CONSTRAINT users_language_check;
ALTER TABLE users ADD CONSTRAINT users_language_check
    CHECK (language IN ('en', 'pl', 'uk', 'ru', 'be', 'de', 'es'));

-- +goose Down
UPDATE users SET language = 'pl' WHERE language IN ('be', 'de', 'es');
ALTER TABLE users DROP CONSTRAINT users_language_check;
ALTER TABLE users ADD CONSTRAINT users_language_check
    CHECK (language IN ('en', 'pl', 'uk', 'ru'));
