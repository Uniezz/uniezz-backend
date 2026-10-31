package feed

import (
	"context"

	"github.com/google/uuid"
)

const (
	TargetPost    = "post"
	TargetComment = "comment"
)

// ReportTarget returns the author of a post or live comment and a copy of its
// text, kept with a report so moderators see what was reported even after an
// edit or delete.
func (m Module) ReportTarget(ctx context.Context, targetType string, id uuid.UUID) (uuid.UUID, map[string]any, error) {
	switch targetType {
	case TargetPost:
		return m.posts.reportTarget(ctx, id)
	case TargetComment:
		return m.comments.reportTarget(ctx, id)
	}
	return uuid.Nil, nil, ErrNotFound
}
