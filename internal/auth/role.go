package auth

import "slices"

type Role string

const (
	RoleModerator Role = "moderator"
	RoleAdmin     Role = "admin"
)

func (p principal) authorize(required []string) error {
	if len(required) == 0 {
		return nil
	}
	for _, role := range p.roles {
		if slices.Contains(required, string(role)) {
			return nil
		}
	}
	return ErrForbidden
}
