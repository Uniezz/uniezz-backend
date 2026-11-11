package study

import (
	"strings"
	"unicode/utf8"

	"github.com/Uniezz/uniezz-backend/internal/text"
)

const (
	minNameLength          = 3
	maxNameLength          = 150
	maxDescriptionLength   = 5000
	maxCommentLength       = 1000
	maxProfessorNameLength = 200
	maxDegreeLength        = 100
)

func cleanName(s string) (string, error) {
	clean := text.Plain(s, false)
	if n := utf8.RuneCountInString(clean); n < minNameLength || n > maxNameLength {
		return "", ErrInvalidInput
	}
	return clean, nil
}

func cleanDescription(s string) (string, error) {
	clean := text.Plain(s, true)
	if n := utf8.RuneCountInString(clean); n == 0 || n > maxDescriptionLength {
		return "", ErrInvalidInput
	}
	return clean, nil
}

func cleanComment(s string) (string, error) {
	clean := text.Plain(s, true)
	if n := utf8.RuneCountInString(clean); n == 0 || n > maxCommentLength {
		return "", ErrInvalidInput
	}
	return clean, nil
}

func cleanFilterValue(s string) (string, error) {
	clean := text.Plain(s, false)
	if utf8.RuneCountInString(clean) > maxNameLength {
		return "", ErrInvalidInput
	}
	return clean, nil
}

func cleanProfessorName(parts ...string) (string, bool) {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if clean := text.Plain(part, false); clean != "" {
			kept = append(kept, clean)
		}
	}
	name := strings.Join(kept, " ")
	if n := utf8.RuneCountInString(name); n == 0 || n > maxProfessorNameLength {
		return "", false
	}
	return name, true
}

func cleanDegree(s string) *string {
	clean := text.Plain(s, false)
	if n := utf8.RuneCountInString(clean); n == 0 || n > maxDegreeLength {
		return nil
	}
	return &clean
}
