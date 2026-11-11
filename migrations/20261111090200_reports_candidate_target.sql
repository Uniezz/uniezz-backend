-- +goose Up
ALTER TABLE reports DROP CONSTRAINT reports_target_type_check;
ALTER TABLE reports ADD CONSTRAINT reports_target_type_check
    CHECK (target_type IN ('post', 'comment', 'user', 'candidate'));

-- +goose Down
DELETE FROM reports WHERE target_type = 'candidate';
ALTER TABLE reports DROP CONSTRAINT reports_target_type_check;
ALTER TABLE reports ADD CONSTRAINT reports_target_type_check
    CHECK (target_type IN ('post', 'comment', 'user'));
