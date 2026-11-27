package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	announceNewcomerTimeout  = 30 * time.Second
	announceNewcomerAttempts = 5
)

type AnnounceNewcomerArgs struct {
	UserID uuid.UUID `json:"userId"`
}

func (AnnounceNewcomerArgs) Kind() string { return "profile.announce-newcomer.v1" }

func (m Module) announceNewcomer(ctx context.Context, args AnnounceNewcomerArgs) error {
	n, err := m.profiles.getNewcomer(ctx, args.UserID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return m.newcomers.Send(ctx, newcomerText(n))
}

func newcomerText(n *newcomerRow) string {
	name := strings.TrimSpace(strings.Join([]string{deref(n.FirstName), deref(n.LastName)}, " "))
	if name == "" {
		name = "(no name yet)"
	}
	return fmt.Sprintf("👋 New student: %s · %s · #%d", name, strings.ToUpper(n.UniversityID), n.Number)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
