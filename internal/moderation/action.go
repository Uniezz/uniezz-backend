package moderation

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

type ActionType string

const (
	ActionReviewReport          ActionType = "review_report"
	ActionDismissReport         ActionType = "dismiss_report"
	ActionRemovePost            ActionType = "remove_post"
	ActionRemoveComment         ActionType = "remove_comment"
	ActionRemoveFile            ActionType = "remove_file"
	ActionRemoveCourse          ActionType = "remove_course"
	ActionRemoveCourseComment   ActionType = "remove_course_comment"
	ActionRemoveProfessorReview ActionType = "remove_professor_review"
	ActionGrantModerator        ActionType = "grant_moderator"
	ActionRevokeModerator       ActionType = "revoke_moderator"
	ActionBan                   ActionType = "ban"
	ActionUnban                 ActionType = "unban"
)

type actionQuery struct {
	Cursor string
	Limit  int
}

func (m Module) listActions(ctx context.Context, q actionQuery) ([]Action, *string, error) {
	filter := actionFilter{Limit: q.Limit + 1}
	if q.Cursor != "" {
		var after actionCursor
		if err := httpapi.DecodeCursor(q.Cursor, &after); err != nil {
			return nil, nil, err
		}
		filter.After = &after
	}

	rows, err := m.actions.list(ctx, filter)
	if err != nil {
		return nil, nil, err
	}

	rows, next, err := httpapi.Page(rows, q.Limit, func(last actionRow) any {
		return actionCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	})
	if err != nil {
		return nil, nil, err
	}

	items := make([]Action, 0, len(rows))
	for _, row := range rows {
		items = append(items, Action(row))
	}
	return items, next, nil
}
