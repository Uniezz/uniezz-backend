package auth

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type Identity struct {
	UniversityID UniversityID
	UsosUserID   string
	Email        string
	FirstName    string
	LastName     string
	ReferralCode string
	Profile      InitialProfile
}

type InitialProfile struct {
	Programmes []InitialProgramme
	PhotoURL   string
	InviterID  *uuid.UUID
}

type InitialProgramme struct {
	Faculty       string
	Programme     string
	Level         string
	ModeOfStudies string
	IsPrimary     bool
}

func (i Identity) normalized() Identity {
	i.Email = normalizeEmail(i.Email)
	i.FirstName = strings.TrimSpace(i.FirstName)
	i.LastName = strings.TrimSpace(i.LastName)
	i.ReferralCode = strings.ToUpper(strings.TrimSpace(i.ReferralCode))
	return i
}

func (i Identity) validateReferral() error {
	if i.ReferralCode != "" && !validInviteCode(i.ReferralCode) {
		return ErrInvalidReferralCode
	}
	return nil
}

func (i Identity) validate(u universityRow) error {
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
