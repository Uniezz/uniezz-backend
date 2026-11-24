-- +goose Up
ALTER TABLE reports DROP CONSTRAINT reports_target_type_check;
ALTER TABLE reports ADD CONSTRAINT reports_target_type_check
    CHECK (target_type IN ('post', 'comment', 'user', 'candidate', 'file', 'course', 'message', 'course_comment', 'professor_review'));

-- +goose Down
DELETE FROM reports WHERE target_type IN ('course_comment', 'professor_review');
ALTER TABLE reports DROP CONSTRAINT reports_target_type_check;
ALTER TABLE reports ADD CONSTRAINT reports_target_type_check
    CHECK (target_type IN ('post', 'comment', 'user', 'candidate', 'file', 'course', 'message'));
