package profile

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/text"
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
		Gender:       p.Gender,
		Language:     p.Language,
		UniversityID: p.UniversityID,
		Programmes:   make([]Programme, 0, len(programmes)),
		AvatarURL:    m.mediaURL(ctx, p.Avatar),
		BannerURL:    m.mediaURL(ctx, p.Banner),
		Bio:          p.Bio,
		Interests:    text.NonNil(p.Interests),
		Privacy:      p.Privacy.withDefaults(),
		CreatedAt:    p.CreatedAt,
	}
	for _, row := range programmes {
		out.Programmes = append(out.Programmes, toProgramme(row))
	}
	return out, nil
}

type Gender string

const (
	GenderMale   Gender = "male"
	GenderFemale Gender = "female"
)

type Language string

const (
	LanguageEnglish   Language = "en"
	LanguagePolish    Language = "pl"
	LanguageUkrainian Language = "uk"
	LanguageRussian   Language = "ru"
)

type profileChange struct {
	FirstName     *string
	LastName      *string
	Gender        *Gender
	Language      *Language
	Bio           *string
	Interests     *[]string
	Privacy       Privacy
	AvatarMediaID *uuid.UUID
	BannerMediaID *uuid.UUID
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
	f.Language = c.Language
	f.Privacy = c.Privacy
	return f, nil
}

func (m Module) updateProfile(ctx context.Context, userID uuid.UUID, change profileChange) error {
	fields, err := change.fields()
	if err != nil {
		return err
	}

	avatarID, err := m.claimMedia(ctx, userID, avatarSlot, change.AvatarMediaID)
	if err != nil {
		return err
	}
	bannerID, err := m.claimMedia(ctx, userID, bannerSlot, change.BannerMediaID)
	if err != nil {
		return err
	}

	var previousAvatar, previousBanner *uuid.UUID
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		if change.Gender != nil {
			if err := m.profiles.setGender(ctx, userID, *change.Gender); err != nil {
				return err
			}
		}
		if err := m.profiles.update(ctx, userID, fields); err != nil {
			return err
		}
		var err error
		if avatarID != nil {
			if previousAvatar, err = m.swapMedia(ctx, userID, avatarSlot, avatarID); err != nil {
				return err
			}
		}
		if bannerID != nil {
			if previousBanner, err = m.swapMedia(ctx, userID, bannerSlot, bannerID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	m.deleteMedia(ctx, previousAvatar)
	m.deleteMedia(ctx, previousBanner)
	return nil
}

func trimmedName(name *string) (*string, error) {
	if name == nil {
		return nil, nil
	}
	trimmed := text.Plain(*name, false)
	if trimmed == "" {
		return nil, ErrInvalidInput
	}
	return &trimmed, nil
}
