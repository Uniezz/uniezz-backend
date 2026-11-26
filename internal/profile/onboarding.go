package profile

import (
	"context"

	"github.com/google/uuid"
)

func (m Module) completeOnboarding(ctx context.Context, userID uuid.UUID) error {
	var onboarded bool
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		onboarded, err = m.profiles.markOnboarded(ctx, userID)
		if err != nil || !onboarded {
			return err
		}
		return m.jobs.Enqueue(ctx, AnnounceNewcomerArgs{UserID: userID})
	})
	if err != nil {
		return err
	}

	if onboarded {
		m.jobs.Wake()
	}
	return nil
}
