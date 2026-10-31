package profile

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

func (m Module) profile(ctx context.Context, userID uuid.UUID) (*Profile, error) {
	p, err := m.profiles.get(ctx, userID)
	if err != nil {
		return nil, err
	}
	programmes, err := m.programmes.list(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := &Profile{
		ID:           p.ID,
		Email:        p.Email,
		FirstName:    p.FirstName,
		LastName:     p.LastName,
		UniversityID: p.UniversityID,
		Programmes:   make([]Programme, 0, len(programmes)),
		AvatarURL:    m.avatarURL(ctx, p.AvatarMediaID),
		CreatedAt:    p.CreatedAt,
	}
	for _, row := range programmes {
		out.Programmes = append(out.Programmes, toProgramme(row))
	}
	return out, nil
}

type profileChange struct {
	FirstName     *string
	LastName      *string
	AvatarMediaID *uuid.UUID
}

func (m Module) updateProfile(ctx context.Context, userID uuid.UUID, change profileChange) error {
	firstName, err := trimmedName(change.FirstName)
	if err != nil {
		return err
	}
	lastName, err := trimmedName(change.LastName)
	if err != nil {
		return err
	}

	var avatarID *uuid.UUID
	if change.AvatarMediaID != nil {
		if avatarID, err = m.claimAvatar(ctx, userID, *change.AvatarMediaID); err != nil {
			return err
		}
	}

	var previous *uuid.UUID
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.profiles.updateNames(ctx, userID, firstName, lastName); err != nil {
			return err
		}
		if avatarID == nil {
			return nil
		}
		var err error
		previous, err = m.swapAvatar(ctx, userID, avatarID)
		return err
	})
	if err != nil {
		return err
	}

	m.deleteMedia(ctx, previous)
	return nil
}

func trimmedName(name *string) (*string, error) {
	if name == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*name)
	if trimmed == "" {
		return nil, ErrInvalidInput
	}
	return &trimmed, nil
}
