package profile

import (
	"unicode/utf8"

	"github.com/Uniezz/uniezz-backend/internal/text"
)

const (
	maxBioLength      = 500
	maxInterests      = 10
	maxInterestLength = 30
)

func cleanBio(bio *string) (*string, bool, error) {
	if bio == nil {
		return nil, false, nil
	}
	clean := text.Plain(*bio, true)
	if utf8.RuneCountInString(clean) > maxBioLength {
		return nil, false, ErrInvalidInput
	}
	if clean == "" {
		return nil, true, nil
	}
	return &clean, true, nil
}

func cleanInterests(interests *[]string) ([]string, error) {
	if interests == nil {
		return nil, nil
	}
	clean, err := text.Tags(*interests, maxInterests, maxInterestLength)
	if err != nil {
		return nil, ErrInvalidInput
	}
	return clean, nil
}
