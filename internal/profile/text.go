package profile

import (
	"strings"
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

	clean := make([]string, 0, len(*interests))
	seen := map[string]bool{}
	for _, raw := range *interests {
		tag := text.Plain(raw, false)
		if tag == "" || utf8.RuneCountInString(tag) > maxInterestLength {
			return nil, ErrInvalidInput
		}
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		clean = append(clean, tag)
	}
	if len(clean) > maxInterests {
		return nil, ErrInvalidInput
	}
	return clean, nil
}
