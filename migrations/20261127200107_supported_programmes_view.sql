-- +goose Up
CREATE VIEW supported_programmes AS
SELECT p.id, p.university_id, p.name, p.level, p.active, f.name AS faculty_name
FROM programmes p
LEFT JOIN supported_faculties f ON f.id = p.faculty_id;

-- +goose Down
DROP VIEW supported_programmes;
