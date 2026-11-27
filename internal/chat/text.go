package chat

import (
	"fmt"
	"regexp"
	"unicode/utf8"

	"github.com/Uniezz/uniezz-backend/internal/text"
)

const (
	maxBodyLength   = 4000
	maxAttachments  = 5
	minNameLength   = 1
	maxNameLength   = 80
	maxGroupMembers = 50
)

func cleanBody(body string, attachments int) (string, error) {
	clean := text.Plain(body, true)
	length := utf8.RuneCountInString(clean)
	if length > maxBodyLength || (length == 0 && attachments == 0) {
		return "", ErrInvalidInput
	}
	return clean, nil
}

var linkPattern = regexp.MustCompile(`(?i)https?://|www\.|\b[a-z0-9-]+\.[a-z]{2,}\b`)

func hasLink(body string) bool {
	return linkPattern.MatchString(body)
}

func cleanName(name string) (string, error) {
	clean := text.Plain(name, false)
	length := utf8.RuneCountInString(clean)
	if length < minNameLength || length > maxNameLength {
		return "", ErrInvalidInput
	}
	return clean, nil
}

func allowedText(field, s string) error {
	if err := text.Allowed(field, s); err != nil {
		return fmt.Errorf("%w: %w", ErrContentNotAllowed, err)
	}
	return nil
}
