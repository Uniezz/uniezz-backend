-- +goose Up
ALTER TABLE moderation_actions DROP CONSTRAINT moderation_actions_type_check;
ALTER TABLE moderation_actions ADD CONSTRAINT moderation_actions_type_check CHECK (type IN (
        'review_report', 'dismiss_report', 'remove_post', 'remove_comment', 'remove_file', 'remove_course',
        'remove_course_comment', 'remove_professor_review',
        'grant_moderator', 'revoke_moderator', 'ban', 'unban'
    ));

-- +goose Down
DELETE FROM moderation_actions WHERE type IN ('remove_course_comment', 'remove_professor_review');
ALTER TABLE moderation_actions DROP CONSTRAINT moderation_actions_type_check;
ALTER TABLE moderation_actions ADD CONSTRAINT moderation_actions_type_check CHECK (type IN (
        'review_report', 'dismiss_report', 'remove_post', 'remove_comment', 'remove_file', 'remove_course',
        'grant_moderator', 'revoke_moderator', 'ban', 'unban'
    ));
