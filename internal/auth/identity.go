package auth

import (
	"errors"
	"strings"
)

var ErrInvalidIdentity = errors.New("invalid identity")

type Identity struct {
	UniversityID UniversityID
	UsosUserID   string
	Email        string
	FirstName    string
	LastName     string
}

func (i Identity) validate() error {
	if i.UniversityID == "" {
		return errors.Join(ErrInvalidIdentity, errors.New("university is required"))
	}
	if i.UsosUserID == "" && i.Email == "" {
		return errors.Join(ErrInvalidIdentity, errors.New("usos user id or email is required"))
	}
	return nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
