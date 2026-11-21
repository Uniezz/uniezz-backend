package moderation

import (
	"context"

	"github.com/google/uuid"
)

func (m Module) removeContent(ctx context.Context, actorID uuid.UUID, targetType string, id uuid.UUID) error {
	var action ActionType
	var err error
	switch targetType {
	case targetPost:
		action, err = ActionRemovePost, m.feed.RemovePost(ctx, id)
	case targetComment:
		action, err = ActionRemoveComment, m.feed.RemoveComment(ctx, id)
	case targetFile:
		action, err = ActionRemoveFile, m.study.RemoveFile(ctx, id)
	case targetCourse:
		action, err = ActionRemoveCourse, m.study.RemoveCourse(ctx, id)
	default:
		return ErrInvalidInput
	}
	if err != nil {
		return err
	}
	if err := m.queue.resolveTarget(ctx, targetType, id, ReportReviewed); err != nil {
		return err
	}
	return m.actions.create(ctx, actorID, action, id)
}
