package profile

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var _ auth.AccountSeeder = Module{}

func (m Module) SeedAccount(ctx context.Context, tx pgx.Tx, userID uuid.UUID, seed auth.AccountSeed) (func(), error) {
	if err := m.programmes.insertSeeded(ctx, tx, userID, seed.Programmes); err != nil {
		return nil, err
	}
	if seed.PhotoURL == "" {
		return nil, nil
	}

	if err := jobs.Enqueue(ctx, tx, ImportAvatar, ImportAvatarArgs{UserID: userID, SourceURL: seed.PhotoURL}); err != nil {
		return nil, err
	}
	return m.jobs.Wake, nil
}
