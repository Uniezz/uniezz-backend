package connections

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
)

type preferencesChange struct {
	BirthDate *string
	OpenTo    []Mode
	Intent    *string
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

func (m Module) savePreferences(ctx context.Context, userID uuid.UUID, c preferencesChange) (*Preferences, error) {
	today := time.Now().UTC()
	f, err := c.fields(today)
	if err != nil {
		return nil, err
	}

	var saved *preferencesRow
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		current, err := m.preferences.lockOrCreate(ctx, userID)
		if err != nil {
			return err
		}
		birthDate, openTo := current.BirthDate, current.OpenTo
		if f.BirthDate != nil {
			birthDate = f.BirthDate
		}
		if f.OpenTo != nil {
			openTo = f.OpenTo
		}
		if slices.Contains(openTo, ModeDating) && (birthDate == nil || ageOn(*birthDate, today) < adultAge) {
			return ErrDatingRequiresAdult
		}
		saved, err = m.preferences.update(ctx, userID, f)
		return err
	})
	if err != nil {
		return nil, err
	}
	return saved.preferences(), nil
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
