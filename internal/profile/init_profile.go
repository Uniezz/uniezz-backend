package profile

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var _ auth.ProfileInitializer = Module{}

func (m Module) InitProfile(ctx context.Context, tx pgx.Tx, userID uuid.UUID, initial auth.InitialProfile) (func(), error) {
	if err := m.programmes.insertInitial(ctx, tx, userID, initial.Programmes); err != nil {
		return nil, err
	}
	if initial.PhotoURL == "" {
		return nil, nil
	}

	if err := jobs.Enqueue(ctx, tx, ImportAvatarArgs{UserID: userID, SourceURL: initial.PhotoURL}); err != nil {
		return nil, err
	}
	return m.jobs.Wake, nil
}
