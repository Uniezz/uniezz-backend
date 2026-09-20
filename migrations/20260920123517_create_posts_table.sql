-- +goose Up
CREATE TABLE posts (
  id    UUID PRIMARY KEY,
  title VARCHAR(150) NOT NULL,
  body  TEXT NOT NULL
);

-- +goose Down
DROP TABLE posts;
