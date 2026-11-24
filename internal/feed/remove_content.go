package feed

import (
	"context"

	"github.com/google/uuid"
)

func (m Module) RemovePost(ctx context.Context, postID uuid.UUID) error {
	return ignoreGone(m.removePost(ctx, postID, nil))
}

func (m Module) RemoveComment(ctx context.Context, commentID uuid.UUID) error {
	return ignoreGone(m.removeComment(ctx, commentID, nil))
}
