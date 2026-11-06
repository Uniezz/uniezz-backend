package auth

import (
	"context"

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
