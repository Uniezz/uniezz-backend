package chat

import (
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

func cleanName(name string) (string, error) {
	clean := text.Plain(name, false)
	length := utf8.RuneCountInString(clean)
	if length < minNameLength || length > maxNameLength {
		return "", ErrInvalidInput
	}
	return clean, nil
}
