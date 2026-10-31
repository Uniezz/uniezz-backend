package profile

import (
	"context"

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
		Bio:          p.Bio,
		Interests:    nonNil(p.Interests),
		Privacy:      p.Privacy.withDefaults(),
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
	Bio           *string
	Interests     *[]string
	Privacy       Privacy
	AvatarMediaID *uuid.UUID
}

func (c profileChange) fields() (profileFields, error) {
	var f profileFields
	var err error
	if f.FirstName, err = trimmedName(c.FirstName); err != nil {
		return profileFields{}, err
	}
	if f.LastName, err = trimmedName(c.LastName); err != nil {
		return profileFields{}, err
	}
	if f.Bio, f.SetBio, err = cleanBio(c.Bio); err != nil {
		return profileFields{}, err
	}
	if f.Interests, err = cleanInterests(c.Interests); err != nil {
		return profileFields{}, err
	}
	for name := range c.Privacy {
		if _, ok := privacySettings[name]; !ok {
			return profileFields{}, ErrInvalidInput
		}
	}
	f.Privacy = c.Privacy
	return f, nil
}

func (m Module) updateProfile(ctx context.Context, userID uuid.UUID, change profileChange) error {
	fields, err := change.fields()
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
		if err := m.profiles.update(ctx, userID, fields); err != nil {
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

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func trimmedName(name *string) (*string, error) {
	if name == nil {
		return nil, nil
	}
	trimmed := plainText(*name, false)
	if trimmed == "" {
		return nil, ErrInvalidInput
	}
	return &trimmed, nil
}
