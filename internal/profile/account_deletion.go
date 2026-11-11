package profile

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const recentSignIn = 15 * time.Minute

func (m Module) deleteAccount(ctx context.Context, userID uuid.UUID, signedInAt time.Time) error {
	if m.now().Sub(signedInAt) > recentSignIn {
		return ErrReauthRequired
	}

	var started bool
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		started, err = m.profiles.markDeleting(ctx, userID)
		if err != nil || !started {
			return err
		}
		return m.jobs.Enqueue(ctx, DeleteAccountArgs{UserID: userID})
	})
	if err != nil {
		return err
	}

	if started {
		m.jobs.Wake()
	}
	return nil
}
