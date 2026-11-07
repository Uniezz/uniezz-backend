package account

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const recentSignIn = 15 * time.Minute

// requestDeletion starts deleting the user's account when they signed in
// recently, so a stolen or forgotten session cannot delete it. Marking the
// user and enqueuing the cleanup commit together: a deletion never starts
// without its job. A second request while one runs succeeds without a second
// job.

func (m Module) requestDeletion(ctx context.Context, userID uuid.UUID, signedInAt time.Time) error {
	if m.now().Sub(signedInAt) > recentSignIn {
		return ErrReauthenticationRequired
	}

	var started bool
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		started, err = m.accounts.BeginDeletion(ctx, userID)
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
