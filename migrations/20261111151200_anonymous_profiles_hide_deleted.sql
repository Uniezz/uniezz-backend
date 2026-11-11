-- +goose Up
CREATE OR REPLACE VIEW anonymous_profiles AS
SELECT u.id, u.university_id, u.gender, u.bio, u.interests,
    sp.level, sp.programme_name AS programme,
    CASE WHEN u.privacy->>'showFaculty' IS DISTINCT FROM 'false' THEN sp.faculty_name END AS faculty
FROM users u
LEFT JOIN student_programmes sp ON sp.user_id = u.id AND sp.is_primary
WHERE u.deleted_at IS NULL;

-- +goose Down
CREATE OR REPLACE VIEW anonymous_profiles AS
SELECT u.id, u.university_id, u.gender, u.bio, u.interests,
    sp.level, sp.programme_name AS programme,
    CASE WHEN u.privacy->>'showFaculty' IS DISTINCT FROM 'false' THEN sp.faculty_name END AS faculty
FROM users u
LEFT JOIN student_programmes sp ON sp.user_id = u.id AND sp.is_primary;
