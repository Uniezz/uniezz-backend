package profile

import (
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/Uniezz/uniezz-backend/internal/text"
)

const (
	maxBioLength      = 500
	maxSongURLLength  = 500
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

func cleanSongURL(raw *string) (*string, bool, error) {
	if raw == nil {
		return nil, false, nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil, true, nil
	}
	if utf8.RuneCountInString(trimmed) > maxSongURLLength {
		return nil, false, ErrInvalidInput
	}
	parsed, err := url.ParseRequestURI(trimmed)
	schemeOK := err == nil && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https"))
	if !schemeOK || parsed.Host == "" {
		return nil, false, ErrInvalidInput
	}
	return &trimmed, true, nil
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
