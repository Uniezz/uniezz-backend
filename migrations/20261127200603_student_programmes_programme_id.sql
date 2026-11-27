-- +goose Up
ALTER TABLE student_programmes
    ADD COLUMN programme_id UUID REFERENCES programmes(id) ON DELETE SET NULL,
    ALTER COLUMN faculty_name DROP NOT NULL;

CREATE INDEX idx_student_programmes_programme_id ON student_programmes (programme_id);

-- +goose Down
DROP INDEX idx_student_programmes_programme_id;

UPDATE student_programmes SET faculty_name = '' WHERE faculty_name IS NULL;

ALTER TABLE student_programmes
    ALTER COLUMN faculty_name SET NOT NULL,
    DROP COLUMN programme_id;
