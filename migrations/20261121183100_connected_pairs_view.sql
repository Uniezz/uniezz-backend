-- +goose Up
CREATE VIEW connected_pairs AS
SELECT user_low AS user_id, user_high AS other_id FROM connections
UNION ALL
SELECT user_high, user_low FROM connections;

-- +goose Down
DROP VIEW connected_pairs;
