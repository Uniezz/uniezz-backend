package profile

import (
	"html"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"
)

const (
	maxBioLength      = 500
	maxInterests      = 10
	maxInterestLength = 30
)

var stripTags = bluemonday.StrictPolicy()

func plainText(s string, keepNewlines bool) string {
	s = html.UnescapeString(stripTags.Sanitize(s))
	s = strings.Map(func(r rune) rune {
		if r == '\n' && keepNewlines {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

func cleanBio(bio *string) (*string, bool, error) {
	if bio == nil {
		return nil, false, nil
	}
	clean := plainText(*bio, true)
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
		tag := plainText(raw, false)
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
