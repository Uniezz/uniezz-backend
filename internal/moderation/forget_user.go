package moderation

import (
	"context"

	"github.com/google/uuid"
)

func (m Module) ForgetUser(ctx context.Context, userID uuid.UUID) error {
	return m.reports.markTargetForgotten(ctx, userID)
}
