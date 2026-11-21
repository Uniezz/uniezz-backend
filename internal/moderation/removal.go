package moderation

import (
	"context"

	"github.com/google/uuid"
)

func (m Module) removeContent(ctx context.Context, targetType string, id uuid.UUID) error {
	var err error
	switch targetType {
	case targetPost:
		err = m.feed.RemovePost(ctx, id)
	case targetComment:
		err = m.feed.RemoveComment(ctx, id)
	case targetFile:
		err = m.study.RemoveFile(ctx, id)
	case targetCourse:
		err = m.study.RemoveCourse(ctx, id)
	default:
		return ErrInvalidInput
	}
	if err != nil {
		return err
	}
	return m.queue.resolveTarget(ctx, targetType, id, ReportReviewed)
}
