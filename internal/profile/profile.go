package profile

import (
	"context"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/rewards"
	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
)

func (m Module) profile(ctx context.Context, userID uuid.UUID, roles []auth.Role) (*Profile, error) {
	p, err := m.profiles.get(ctx, userID)
	if err != nil {
		return nil, err
	}
	connectionPreferences, err := m.profiles.getConnectionPreferences(ctx, userID)
	if err != nil {
		return nil, err
	}
	programmes, err := m.programmes.list(ctx, userID)
	if err != nil {
		return nil, err
	}
	coins, err := m.rewards.Balance(ctx, userID)
	if err != nil {
		return nil, err
	}

	var birthDate *string
	if connectionPreferences.BirthDate != nil {
		value := connectionPreferences.BirthDate.Format(time.DateOnly)
		birthDate = &value
	}

	openTo := connectionPreferences.OpenTo

	out := &Profile{
		ID:           p.ID,
		Email:        p.Email,
		FirstName:    p.FirstName,
		LastName:     p.LastName,
		Gender:       p.Gender,
		BirthDate:    birthDate,
		OpenTo:       openTo,
		Intent:       connectionPreferences.Intent,
		Language:     p.Language,
		UniversityID: p.UniversityID,
		Programmes:   make([]Programme, 0, len(programmes)),
		AvatarURL:    m.mediaURL(ctx, p.Avatar),
		BannerURL:    m.mediaURL(ctx, p.Banner),
		Bio:          p.Bio,
		SongURL:      p.SongURL,
		Interests:    text.NonNil(p.Interests),
		Privacy:      p.Privacy.withDefaults(),
		Coins:        coins,
		Roles:        text.NonNil(roles),
		OnboardedAt:  p.OnboardedAt,
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
	LanguageEnglish    Language = "en"
	LanguagePolish     Language = "pl"
	LanguageUkrainian  Language = "uk"
	LanguageRussian    Language = "ru"
	LanguageBelarusian Language = "be"
	LanguageGerman     Language = "de"
	LanguageSpanish    Language = "es"
)

type profileChange struct {
	FirstName        *string
	LastName         *string
	Gender           *Gender
	BirthDate        *string
	BirthDatePresent bool
	OpenTo           *[]string
	Intent           *string
	Language         *Language
	Bio              *string
	SongURL          *string
	Interests        *[]string
	Privacy          Privacy
	AvatarMediaID    *uuid.UUID
	BannerMediaID    *uuid.UUID
}

func (c profileChange) fields() (profileFields, error) {
	var f profileFields
	var err error
	if f.FirstName, err = trimmedName("firstName", c.FirstName); err != nil {
		return profileFields{}, err
	}
	if f.LastName, err = trimmedName("lastName", c.LastName); err != nil {
		return profileFields{}, err
	}
	if f.Bio, f.SetBio, err = cleanBio(c.Bio); err != nil {
		return profileFields{}, err
	}
	if f.SongURL, f.SetSongURL, err = cleanSongURL(c.SongURL); err != nil {
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

func (m Module) updateProfile(ctx context.Context, userID uuid.UUID, change profileChange) (int, error) {
	fields, err := change.fields()
	if err != nil {
		text.ReportNotAllowed(ctx, userID, err)
		return 0, err
	}

	var avatarID, bannerID *uuid.UUID
	var previousAvatar, previousBanner *uuid.UUID
	earned := 0

	avatarID, err = m.claimMedia(ctx, userID, avatarSlot, change.AvatarMediaID)
	if err != nil {
		return 0, err
	}
	bannerID, err = m.claimMedia(ctx, userID, bannerSlot, change.BannerMediaID)
	if err != nil {
		return 0, err
	}

	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		if change.AvatarMediaID != nil {
			if err := m.profiles.checkAvatarPhotoLimit(ctx, userID); err != nil {
				return err
			}
		}

		if change.Gender != nil {
			if err := m.profiles.setGender(ctx, userID, *change.Gender); err != nil {
				return err
			}
		}
		if err := m.profiles.update(ctx, userID, fields); err != nil {
			return err
		}
		if change.BirthDatePresent || change.OpenTo != nil || change.Intent != nil {
			connectionEarned, err := m.connections.UpdateConnectionPreferences(ctx, userID, ConnectionPreferencesChange{
				BirthDate:        change.BirthDate,
				BirthDatePresent: change.BirthDatePresent,
				OpenTo:           change.OpenTo,
				Intent:           change.Intent,
			})
			if err != nil {
				return err
			}
			earned += connectionEarned
		}

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
		complete, err := m.profiles.isComplete(ctx, userID)
		if err != nil || !complete {
			return err
		}
		profileCompleteEarned, err := m.rewards.Grant(ctx, userID, rewards.ReasonProfileComplete, nil, nil)
		if err != nil {
			return err
		}
		earned += profileCompleteEarned
		return nil
	})
	if err != nil {
		return 0, err
	}

	m.deleteMedia(ctx, previousAvatar)
	m.deleteMedia(ctx, previousBanner)
	return earned, nil
}

func trimmedName(field string, name *string) (*string, error) {
	if name == nil {
		return nil, nil
	}
	trimmed := text.Plain(*name, false)
	if trimmed == "" {
		return nil, ErrInvalidInput
	}
	if err := text.Allowed(field, trimmed); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrContentNotAllowed, err)
	}
	return &trimmed, nil
}
