-- +goose Up
ALTER TABLE courses ADD COLUMN faculty_id UUID REFERENCES university_faculties(id);

-- Courses named their faculty in free text: take the faculty of the same
-- university whose name matches exactly, else the most similar one.
UPDATE courses c SET faculty_id = (
    SELECT f.id
    FROM university_faculties f
    WHERE f.university_id = c.university_id
    ORDER BY public.search_text(f.name) = public.search_text(c.faculty) DESC,
        public.similarity(public.search_text(f.name), public.search_text(c.faculty)) DESC,
        f.position, f.id
    LIMIT 1
);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM courses WHERE faculty_id IS NULL) THEN
        RAISE EXCEPTION 'courses of a university without faculties cannot be migrated';
    END IF;
END
$$;
-- +goose StatementEnd

ALTER TABLE courses ALTER COLUMN faculty_id SET NOT NULL;

DROP INDEX idx_courses_subject_search;
DROP INDEX idx_courses_university_id_faculty_subject_semester;
ALTER TABLE courses
    DROP COLUMN faculty,
    DROP COLUMN subject,
    DROP COLUMN semester;

CREATE INDEX idx_courses_faculty_id_created_at ON courses (faculty_id, created_at DESC, id DESC);
CREATE INDEX idx_courses_university_id_created_at ON courses (university_id, created_at DESC, id DESC);

-- +goose Down
DROP INDEX idx_courses_university_id_created_at;
DROP INDEX idx_courses_faculty_id_created_at;

ALTER TABLE courses
    ADD COLUMN faculty VARCHAR(150),
    ADD COLUMN subject VARCHAR(150),
    ADD COLUMN semester SMALLINT;

UPDATE courses c SET
    faculty = left(f.name, 150),
    subject = c.name,
    semester = 1
FROM university_faculties f
WHERE f.id = c.faculty_id;

ALTER TABLE courses
    ALTER COLUMN faculty SET NOT NULL,
    ALTER COLUMN subject SET NOT NULL,
    ALTER COLUMN semester SET NOT NULL,
    ADD CONSTRAINT courses_faculty_check CHECK (char_length(faculty) BETWEEN 3 AND 150),
    ADD CONSTRAINT courses_subject_check CHECK (char_length(subject) BETWEEN 3 AND 150),
    ADD CONSTRAINT courses_semester_check CHECK (semester BETWEEN 1 AND 10);

CREATE INDEX idx_courses_university_id_faculty_subject_semester
    ON courses (university_id, faculty, subject, semester, created_at DESC, id DESC);
CREATE INDEX idx_courses_subject_search ON courses USING gin (public.search_text(subject) public.gin_trgm_ops);

ALTER TABLE courses DROP COLUMN faculty_id;
