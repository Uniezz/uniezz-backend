package connections

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/profile"
	"github.com/Uniezz/uniezz-backend/internal/rewards"
	"github.com/google/uuid"
)

type preferencesChange struct {
	BirthDate        *string
	BirthDatePresent bool
	OpenTo           []Mode
	Intent           *string
}

func (c preferencesChange) fields(today time.Time) (preferencesFields, error) {
	f := preferencesFields{SetIntent: c.Intent != nil}
	if c.BirthDate != nil {
		birthDate, err := parseBirthDate(*c.BirthDate, today)
		if err != nil {
			return f, err
		}
		f.BirthDate = birthDate
	}
	if c.OpenTo != nil {
		for _, mode := range c.OpenTo {
			if !mode.valid() {
				return f, ErrInvalidInput
			}
		}
		f.OpenTo = slices.Compact(slices.Sorted(slices.Values(c.OpenTo)))
		if f.OpenTo == nil {
			f.OpenTo = []Mode{}
		}
	}
	intent, err := cleanIntent(c.Intent)
	if err != nil {
		return f, err
	}
	f.Intent = intent
	return f, nil
}

func (m Module) getPreferences(ctx context.Context, userID uuid.UUID) (*Preferences, error) {
	row, err := m.preferences.get(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return &Preferences{OpenTo: []Mode{}}, nil
	}
	if err != nil {
		return nil, err
	}
	return row.preferences(), nil
}

func (m Module) UpdateConnectionPreferences(ctx context.Context, userID uuid.UUID, change profile.ConnectionPreferencesChange) (int, error) {
	var openTo []Mode
	if change.OpenTo != nil {
		openTo = make([]Mode, len(*change.OpenTo))
		for i, mode := range *change.OpenTo {
			openTo[i] = Mode(mode)
		}
	}

	_, earned, err := m.savePreferences(ctx, userID, preferencesChange{
		BirthDate:        change.BirthDate,
		BirthDatePresent: change.BirthDatePresent,
		OpenTo:           openTo,
		Intent:           change.Intent,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrContentNotAllowed):
			return 0, profile.ErrContentNotAllowed
		case errors.Is(err, ErrInvalidInput):
			return 0, profile.ErrInvalidInput
		case errors.Is(err, ErrBirthDateImmutable):
			return 0, profile.ErrBirthDateImmutable
		case errors.Is(err, ErrDatingRequiresProfile):
			return 0, profile.ErrDatingRequiresProfile
		case errors.Is(err, ErrDatingRequiresAdult):
			return 0, profile.ErrDatingRequiresAdult
		default:
			return 0, err
		}
	}
	return earned, nil
}

func (m Module) savePreferences(ctx context.Context, userID uuid.UUID, c preferencesChange) (*Preferences, int, error) {
	today := m.now().UTC()
	f, err := c.fields(today)
	if err != nil {
		return nil, 0, err
	}

	var saved *preferencesRow
	earned := 0
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		current, err := m.preferences.lockOrCreate(ctx, userID)
		if err != nil {
			return err
		}
		if err := allowedIntent(f.Intent, current.Intent); err != nil {
			return err
		}
		if c.BirthDatePresent && c.BirthDate == nil && current.BirthDate != nil {
			return ErrBirthDateImmutable
		}
		if f.BirthDate != nil && current.BirthDate != nil && !f.BirthDate.Equal(*current.BirthDate) {
			return ErrBirthDateImmutable
		}
		birthDate, openTo := current.BirthDate, current.OpenTo
		if f.BirthDate != nil {
			birthDate = f.BirthDate
		}
		if f.OpenTo != nil {
			openTo = f.OpenTo
		}
		if slices.Contains(openTo, ModeDating) {
			if birthDate == nil {
				return ErrDatingRequiresProfile
			}

			summaries, err := m.profiles.Summaries(ctx, []uuid.UUID{userID})
			if err != nil {
				return err
			}
			summary, ok := summaries[userID]
			if !ok || summary.Gender == nil {
				return ErrDatingRequiresProfile
			}
			if ageOn(*birthDate, today) < adultAge {
				return ErrDatingRequiresAdult
			}
		}
		saved, err = m.preferences.update(ctx, userID, f)
		if err != nil || current.BirthDate != nil || f.BirthDate == nil {
			return err
		}
		earned, err = m.rewards.Grant(ctx, userID, rewards.ReasonBirthDate, nil, nil)
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	return saved.preferences(), earned, nil
}

func (row preferencesRow) preferences() *Preferences {
	p := &Preferences{OpenTo: row.OpenTo, Intent: row.Intent}
	if p.OpenTo == nil {
		p.OpenTo = []Mode{}
	}
	if row.BirthDate != nil {
		birthDate := row.BirthDate.Format(time.DateOnly)
		p.BirthDate = &birthDate
	}
	return p
}
