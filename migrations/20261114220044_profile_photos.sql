-- +goose Up
CREATE TABLE profile_photos (
    media_id UUID PRIMARY KEY REFERENCES media(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    position SMALLINT NOT NULL CHECK (position BETWEEN 1 AND 6),
    UNIQUE (user_id, position)
);

-- +goose Down
DROP TABLE profile_photos;