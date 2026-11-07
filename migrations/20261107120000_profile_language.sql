-- +goose Up
ALTER TABLE users ADD COLUMN language TEXT NOT NULL DEFAULT 'pl'
    CHECK (language IN ('en', 'pl', 'uk', 'ru'));

-- +goose Down
ALTER TABLE users DROP COLUMN language;
