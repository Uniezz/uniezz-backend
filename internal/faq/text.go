package faq

import (
	"fmt"
	"unicode/utf8"

	"github.com/Uniezz/uniezz-backend/internal/text"
)

const (
	minQuestionLength = 3
	maxQuestionLength = 200
	minAnswerLength   = 1
	maxAnswerLength   = 2000
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
