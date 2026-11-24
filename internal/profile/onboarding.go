package profile

import (
	"context"

	"github.com/google/uuid"
)

func (m Module) completeOnboarding(ctx context.Context, userID uuid.UUID) error {
	return m.profiles.markOnboarded(ctx, userID)
}
