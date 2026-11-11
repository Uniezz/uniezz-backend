package connections

import (
	"context"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
)

const (
	modeFriends = "friends"
	modeStudy   = "studyBuddy"
	modeDating  = "dating"

	minAge          = 16
	maxAge          = 100
	adultAge        = 18
	maxIntentLength = 200
)

type preferencesChange struct {
	BirthDate *time.Time
	OpenTo    []string
	SetIntent bool
	Intent    *string
}

func (m Module) savePreferences(ctx context.Context, userID uuid.UUID, c preferencesChange) error {
	today := time.Now().UTC()
	if c.BirthDate != nil {
		if age := ageOn(*c.BirthDate, today); age < minAge || age > maxAge {
			return ErrInvalidInput
		}
	}
	for _, mode := range c.OpenTo {
		if !validMode(mode) {
			return ErrInvalidInput
		}
	}
	intent, err := cleanIntent(c.Intent)
	if err != nil {
		return err
	}

	return m.tx.WithTx(ctx, func(ctx context.Context) error {
		current, err := m.preferences.lock(ctx, userID)
		if err != nil {
			return err
		}
		if c.BirthDate != nil {
			current.BirthDate = c.BirthDate
		}
		if c.OpenTo != nil {
			current.OpenTo = text.NonNil(slices.Compact(slices.Sorted(slices.Values(c.OpenTo))))
		}
		if c.SetIntent {
			current.Intent = intent
		}
		if slices.Contains(current.OpenTo, modeDating) && (current.BirthDate == nil || ageOn(*current.BirthDate, today) < adultAge) {
			return ErrDatingRequiresAdult
		}
		return m.preferences.update(ctx, userID, *current)
	})
}

func cleanIntent(intent *string) (*string, error) {
	if intent == nil {
		return nil, nil
	}
	clean := text.Plain(*intent, false)
	if utf8.RuneCountInString(clean) > maxIntentLength {
		return nil, ErrInvalidInput
	}
	if clean == "" {
		return nil, nil
	}
	return &clean, nil
}

func ageOn(birthDate, day time.Time) int {
	age := day.Year() - birthDate.Year()
	if day.Month() < birthDate.Month() || (day.Month() == birthDate.Month() && day.Day() < birthDate.Day()) {
		age--
	}
	return age
}

func validMode(mode string) bool {
	return mode == modeFriends || mode == modeStudy || mode == modeDating
}
