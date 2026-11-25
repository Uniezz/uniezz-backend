package feed

import (
	"fmt"
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

func cleanText(field, s string, keepNewlines bool, minLength, maxLength int) (string, error) {
	clean := text.Plain(s, keepNewlines)
	n := utf8.RuneCountInString(clean)
	if n < minLength || n > maxLength {
		return "", ErrInvalidInput
	}
	if err := text.Allowed(field, clean); err != nil {
		return "", fmt.Errorf("%w: %w", ErrContentNotAllowed, err)
	}
	return clean, nil
}

func cleanTags(tags []string) ([]string, error) {
	clean, err := text.Tags(tags, maxTags, maxTagLength)
	if err != nil {
		return nil, ErrInvalidInput
	}
	for _, tag := range clean {
		if err := text.Allowed("tags", tag); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrContentNotAllowed, err)
		}
	}
	return clean, nil
}
