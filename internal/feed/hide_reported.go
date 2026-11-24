package feed

import (
	"context"

	"github.com/google/uuid"
)

func (m Module) HideReported(ctx context.Context, targetType string, id uuid.UUID) error {
	switch targetType {
	case TargetPost:
		return m.posts.hide(ctx, id)
	case TargetComment:
		return m.comments.hide(ctx, id)
	}
	return nil
}

func (m Module) RestoreReported(ctx context.Context, targetType string, id uuid.UUID) error {
	switch targetType {
	case TargetPost:
		return m.posts.restore(ctx, id)
	case TargetComment:
		return m.comments.restore(ctx, id)
	}
	return nil
}
