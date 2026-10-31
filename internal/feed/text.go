package feed

import (
	"strings"
	"unicode/utf8"

	"github.com/Uniezz/uniezz-backend/internal/text"
)

const (
	minTitleLength   = 3
	maxTitleLength   = 150
	minContentLength = 10
	maxContentLength = 5000
	maxCommentLength = 1000
	maxTags          = 10
	maxTagLength     = 30
)

func cleanText(s string, keepNewlines bool, minLength, maxLength int) (string, error) {
	clean := text.Plain(s, keepNewlines)
	n := utf8.RuneCountInString(clean)
	if n < minLength || n > maxLength {
		return "", ErrInvalidInput
	}
	return clean, nil
}

func cleanTags(tags []string) ([]string, error) {
	clean := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, raw := range tags {
		tag := text.Plain(raw, false)
		if tag == "" || utf8.RuneCountInString(tag) > maxTagLength {
			return nil, ErrInvalidInput
		}
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		clean = append(clean, tag)
	}
	if len(clean) > maxTags {
		return nil, ErrInvalidInput
	}
	return clean, nil
}
