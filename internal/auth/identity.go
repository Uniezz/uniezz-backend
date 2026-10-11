package auth

import (
	"fmt"
	"strings"
)

type Identity struct {
	UniversityID UniversityID
	UsosUserID   string
	Email        string
	FirstName    string
	LastName     string
}

func (i Identity) normalized() Identity {
	i.Email = normalizeEmail(i.Email)
	i.FirstName = strings.TrimSpace(i.FirstName)
	i.LastName = strings.TrimSpace(i.LastName)
	return i
}

func (i Identity) validate() error {
	u, err := GetUniversityByID(i.UniversityID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidIdentity, err)
	}

	switch u.AuthType {
	case AuthTypeUSOS:
		if i.UsosUserID == "" {
			return fmt.Errorf("%w: usos user id is required for %s", ErrInvalidIdentity, u.ID)
		}
	case AuthTypeOTP:
		if i.Email == "" {
			return fmt.Errorf("%w: email is required for %s", ErrInvalidIdentity, u.ID)
		}
	}
	return nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
