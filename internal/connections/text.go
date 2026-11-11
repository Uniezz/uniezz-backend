package connections

import (
	"time"
	"unicode/utf8"

	"github.com/Uniezz/uniezz-backend/internal/text"
)

const (
	minAge          = 16
	maxAge          = 100
	adultAge        = 18
	maxIntentLength = 200
)

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

func parseBirthDate(value string, today time.Time) (*time.Time, error) {
	birthDate, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return nil, ErrInvalidInput
	}
	if age := ageOn(birthDate, today); age < minAge || age > maxAge {
		return nil, ErrInvalidInput
	}
	return &birthDate, nil
}

func ageOn(birthDate, day time.Time) int {
	age := day.Year() - birthDate.Year()
	if day.Month() < birthDate.Month() || (day.Month() == birthDate.Month() && day.Day() < birthDate.Day()) {
		age--
	}
	return age
}
