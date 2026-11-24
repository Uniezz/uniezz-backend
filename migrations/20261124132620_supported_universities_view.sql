-- +goose Up
CREATE VIEW supported_universities AS
SELECT id, name, radon_institution_id
FROM universities;

-- +goose Down
DROP VIEW supported_universities;
