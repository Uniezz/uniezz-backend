package profile

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/google/uuid"
)

var _ auth.ProfileInitializer = Module{}

func (m Module) InitProfile(ctx context.Context, userID uuid.UUID, initial auth.InitialProfile) (func(), error) {
	if err := m.programmes.insertInitial(ctx, userID, initial.Programmes); err != nil {
		return nil, err
	}
	if initial.PhotoURL == "" {
		return nil, nil
	}

	if err := m.jobs.Enqueue(ctx, ImportAvatarArgs{UserID: userID, SourceURL: initial.PhotoURL}); err != nil {
		return nil, err
	}
	return m.jobs.Wake, nil
}
