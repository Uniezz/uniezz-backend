-- +goose Up
CREATE TABLE rate_limits (
    key          TEXT NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    count        INT NOT NULL,
    PRIMARY KEY (key, window_start)
);

-- +goose Down
DROP TABLE rate_limits;
