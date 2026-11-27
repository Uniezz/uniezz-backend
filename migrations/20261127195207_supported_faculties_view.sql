-- +goose Up
CREATE VIEW supported_faculties AS
SELECT id, university_id, name, position
FROM university_faculties;

-- +goose Down
DROP VIEW supported_faculties;
