package auth

import (
	"context"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/usos"
)

type Platform string

const (
	PlatformWeb    Platform = "web"
	PlatformMobile Platform = "mobile"
)

func parsePlatform(s string) (Platform, error) {
	switch p := Platform(s); p {
	case PlatformWeb, PlatformMobile:
		return p, nil
	default:
		return "", fmt.Errorf("%w: unknown platform %q", ErrInvalidInput, s)
	}
}

var _ usosClient = (*usos.Client)(nil) // compile-time check that the real USOS client satisfies the interface usosLogin needs

type usosClient interface {
	Begin(ctx context.Context) (usos.RequestToken, string, error)
	Finish(ctx context.Context, rt usos.RequestToken, verifier string) (usos.User, error)
}

type usosState struct {
	RequestToken usos.RequestToken
	UniversityID UniversityID
	Platform     Platform
}

var _ usosStateStore = (*usosStateRepository)(nil)

type usosStateStore interface {
	save(ctx context.Context, s usosState) error
	take(ctx context.Context, requestToken string) (usosState, error)
}

type usosLogin struct {
	clients map[UniversityID]usosClient
	states  usosStateStore
}

func newUsosLogin(clients map[UniversityID]usosClient, states usosStateStore) *usosLogin {
	return &usosLogin{
		clients: clients,
		states:  states,
	}
}

func (l *usosLogin) start(ctx context.Context, universityID UniversityID, platform Platform) (authorizeURL string, err error) {
	un, err := requireUniversity(universityID, AuthTypeUSOS)
	if err != nil {
		return "", err
	}

	client, ok := l.clients[un.ID]
	if !ok {
		return "", fmt.Errorf("%w: no USOS client for university %q", ErrUpstream, un.ID)
	}

	rt, authorizeURL, err := client.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: begin USOS login for %q: %w", ErrUpstream, un.ID, err)
	}

	err = l.states.save(ctx, usosState{
		RequestToken: rt,
		UniversityID: un.ID,
		Platform:     platform,
	})
	if err != nil {
		return "", fmt.Errorf("save USOS state for %q: %w", un.ID, err)
	}

	return authorizeURL, nil
}

func (l *usosLogin) finish(ctx context.Context, requestToken, verifier string) (Identity, Platform, error) {
	us, err := l.states.take(ctx, requestToken)
	if err != nil {
		return Identity{}, "", err
	}

	client, ok := l.clients[us.UniversityID]
	if !ok {
		return Identity{}, us.Platform, fmt.Errorf("%w: no USOS client for university %q", ErrUpstream, us.UniversityID)
	}

	user, err := client.Finish(ctx, us.RequestToken, verifier)
	if err != nil {
		return Identity{}, us.Platform, fmt.Errorf("%w: finish USOS login for %q: %w", ErrUpstream, us.UniversityID, err)
	}

	if user.StudentStatus == nil {
		return Identity{}, us.Platform, fmt.Errorf("%w: we don't have access to the student_status field for usos user %q", ErrUpstream, user.ID)
	}

	if *user.StudentStatus != usos.StatusActiveStudent {
		return Identity{}, us.Platform, fmt.Errorf("%w: student_status=%d", ErrNotStudent, *user.StudentStatus)
	}

	var email string
	if user.Email != nil {
		email = *user.Email
	}

	return Identity{
		UniversityID: us.UniversityID,
		UsosUserID:   user.ID,
		Email:        email,
		FirstName:    user.FirstName,
		LastName:     user.LastName,
	}, us.Platform, nil
}
