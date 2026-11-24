package push

import (
	"context"

	"github.com/google/uuid"
)

type tokenChange struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	Token     string
}

func (m Module) registerToken(ctx context.Context, c tokenChange) error {
	token, err := cleanToken(c.Token)
	if err != nil {
		return err
	}
	return m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.tokens.deleteOtherOfSession(ctx, c.SessionID, token); err != nil {
			return err
		}
		return m.tokens.upsert(ctx, c.SessionID, c.UserID, token)
	})
}

func (m Module) removeToken(ctx context.Context, sessionID uuid.UUID) error {
	return m.tokens.deleteOfSession(ctx, sessionID)
}
