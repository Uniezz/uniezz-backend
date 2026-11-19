-- +goose Up
CREATE VIEW connection_preferences AS
SELECT user_id, birth_date, open_to, intent
FROM connection_profiles;

-- +goose Down
DROP VIEW connection_preferences;
