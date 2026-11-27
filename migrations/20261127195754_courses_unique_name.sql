-- +goose Up
UPDATE courses SET name = regexp_replace(btrim(name), '\s+', ' ', 'g') WHERE name ~ '\s{2}|^\s|\s$';

-- Courses of one faculty with the same name differed only by their old
-- programme or semester: fold each group into its oldest course.
CREATE TEMPORARY TABLE course_merges ON COMMIT DROP AS
SELECT id, first_value(id) OVER (
    PARTITION BY faculty_id, public.search_text(name) ORDER BY created_at, id
) AS keep_id
FROM courses;

DELETE FROM course_merges WHERE id = keep_id;

UPDATE course_files f SET course_id = m.keep_id FROM course_merges m WHERE f.course_id = m.id;
UPDATE course_comments c SET course_id = m.keep_id FROM course_merges m WHERE c.course_id = m.id;

INSERT INTO course_ratings (course_id, user_id, rating)
SELECT m.keep_id, r.user_id, r.rating
FROM course_ratings r JOIN course_merges m ON m.id = r.course_id
ON CONFLICT (course_id, user_id) DO NOTHING;

DELETE FROM courses c USING course_merges m WHERE c.id = m.id;

UPDATE courses c SET rating_count = r.count, rating_sum = r.sum
FROM (
    SELECT course_id, COUNT(*) AS count, COALESCE(SUM(rating), 0) AS sum
    FROM course_ratings
    WHERE course_id IN (SELECT DISTINCT keep_id FROM course_merges)
    GROUP BY course_id
) r
WHERE c.id = r.course_id;

CREATE UNIQUE INDEX uq_courses_faculty_id_name ON courses (faculty_id, public.search_text(name));

-- +goose Down
DROP INDEX uq_courses_faculty_id_name;
