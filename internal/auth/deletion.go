package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

func (m Module) BeginDeletion(ctx context.Context, userID uuid.UUID) (bool, error) {
	var started bool
	err := m.logins.tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		started, err = m.logins.users.markDeleting(ctx, userID)
		if err != nil || !started {
			return err
		}
		if err := m.logins.sessions.deleteForUser(ctx, userID); err != nil {
			return err
		}
		return m.logins.loginCodes.deleteForUser(ctx, userID)
	})
	return started, err
}

var errStillReferenced = errors.New("the user still has feed content or media; their deletion runs again")

func (m Module) FinishDeletion(ctx context.Context, userID uuid.UUID) error {
	return m.logins.users.deleteDeleting(ctx, userID)
}

func (m Module) StuckDeletions(ctx context.Context, olderThan time.Duration, limit int) ([]uuid.UUID, error) {
	return m.logins.users.listDeletingSince(ctx, olderThan, limit)
}
