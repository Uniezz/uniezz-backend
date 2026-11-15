package profile

import (
	"context"
	"time"
	"unicode/utf8"

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

	var birthDate *string
	if p.BirthDate != nil {
		value := p.BirthDate.Format("2006-01-02")
		birthDate = &value
	}

	openTo := make([]string, len(p.OpenTo))
	for i, mode := range p.OpenTo {
		if mode == "studyBuddy" {
			mode = "study_buddy"
		}
		openTo[i] = mode
	}

	out := &Profile{
		ID:           p.ID,
		Email:        p.Email,
		FirstName:    p.FirstName,
		LastName:     p.LastName,
		Gender:       p.Gender,
		BirthDate:    birthDate,
		OpenTo:       openTo,
		Intent:       p.Intent,
		Language:     p.Language,
		UniversityID: p.UniversityID,
		Programmes:   make([]Programme, 0, len(programmes)),
		AvatarURL:    m.mediaURL(ctx, p.Avatar),
		BannerURL:    m.mediaURL(ctx, p.Banner),
		Bio:          p.Bio,
		SongURL:      p.SongURL,
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
	if f.FirstName, err = trimmedName(c.FirstName); err != nil {
		return profileFields{}, err
	}
	if f.LastName, err = trimmedName(c.LastName); err != nil {
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
	if c.BirthDate != nil {
		birthDate, parseErr := time.Parse(time.DateOnly, *c.BirthDate)
		if parseErr != nil {
			return profileFields{}, ErrInvalidInput
		}

		today := time.Now().UTC()
		age := today.Year() - birthDate.Year()
		if today.Month() < birthDate.Month() ||
			(today.Month() == birthDate.Month() && today.Day() < birthDate.Day()) {
			age--
		}
		if age < 16 || age > 100 {
			return profileFields{}, ErrInvalidInput
		}
		f.BirthDate = c.BirthDate
	}

	f.SetBirthDate = c.BirthDatePresent

	if c.OpenTo != nil {
		f.OpenTo = *c.OpenTo
		f.SetOpenTo = true
	}
	f.SetIntent = c.Intent != nil
	if c.Intent != nil {
		intent := text.Plain(*c.Intent, false)
		if utf8.RuneCountInString(intent) > 150 {
			return profileFields{}, ErrInvalidInput
		}
		if intent != "" {
			f.Intent = &intent
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

	var avatarID, bannerID *uuid.UUID
	var previousAvatar, previousBanner *uuid.UUID

	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		if change.AvatarMediaID != nil {
			if err := m.profiles.checkAvatarPhotoLimit(ctx, userID); err != nil {
				return err
			}
		}

		var err error
		avatarID, err = m.claimMedia(ctx, userID, avatarSlot, change.AvatarMediaID)
		if err != nil {
			return err
		}
		bannerID, err = m.claimMedia(ctx, userID, bannerSlot, change.BannerMediaID)
		if err != nil {
			return err
		}
		if change.Gender != nil {
			if err := m.profiles.setGender(ctx, userID, *change.Gender); err != nil {
				return err
			}
		}
		if err := m.profiles.update(ctx, userID, fields); err != nil {
			return err
		}
		if err := m.profiles.updateConnectionProfile(ctx, userID, fields); err != nil {
			return err
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
