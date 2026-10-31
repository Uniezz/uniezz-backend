package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

const defaultSessionTTL = 30 * 24 * time.Hour

type logins struct {
	tx         database.Transactor
	users      *UserRepository
	sessions   *SessionRepository
	loginCodes *loginCodeRepository
	sessionTTL time.Duration
	profiles   ProfileInitializer
}

type ProfileInitializer interface {
	InitProfile(ctx context.Context, userID uuid.UUID, initial InitialProfile) (afterCommit func(), err error)
}

func newLogins(tx database.Transactor, users *UserRepository, sessions *SessionRepository, loginCodes *loginCodeRepository, sessionTTL time.Duration) *logins {
	return &logins{
		tx:         tx,
		users:      users,
		sessions:   sessions,
		loginCodes: loginCodes,
		sessionTTL: sessionTTL,
	}
}

func (l *logins) issueLoginCode(ctx context.Context, userID uuid.UUID) (string, error) {
	code := GenerateSessionToken()
	hash := code.Hash()

	if err := l.loginCodes.create(ctx, userID, hash[:]); err != nil {
		return "", err
	}
	return code.String(), nil
}

func (l *logins) exchange(ctx context.Context, code string) (IssuedSession, error) {
	hash, err := HashTokenString(code)
	if err != nil {
		return IssuedSession{}, fmt.Errorf("%w: %w", ErrInvalidCode, err)
	}

	userID, err := l.loginCodes.consume(ctx, hash[:])
	if err != nil {
		return IssuedSession{}, err
	}
	return l.issueSession(ctx, userID)
}

func requireUniversity(id UniversityID, method AuthType) (University, error) {
	u, err := GetUniversityByID(id)
	if err != nil {
		return University{}, err
	}
	if u.AuthType != method {
		return University{}, fmt.Errorf("%w: %s uses %s", ErrWrongAuthMethod, u.ID, u.AuthType)
	}
	return u, nil
}

func (l *logins) signIn(ctx context.Context, id Identity) (*User, error) {
	id = id.normalized()
	if err := id.validate(); err != nil {
		return nil, err
	}
	var user *User
	var afterCommit func()
	err := l.tx.WithTx(ctx, func(ctx context.Context) error {
		u, created, err := l.users.upsert(ctx, id)
		if err != nil {
			return err
		}
		user = u
		if !created || l.profiles == nil {
			return nil
		}
		afterCommit, err = l.profiles.InitProfile(ctx, u.ID, id.Profile)
		return err
	})
	if err != nil {
		return nil, err
	}

	if afterCommit != nil {
		afterCommit()
	}
	return user, nil
}

type IssuedSession struct {
	Token     string    `json:"token" doc:"token to be used in Authorization header for subsequent requests"`
	ExpiresAt time.Time `json:"expiresAt" doc:"timestamp when the session expires"`
}

func (l *logins) issueSession(ctx context.Context, userID uuid.UUID) (IssuedSession, error) {
	token := GenerateSessionToken()
	hash := token.Hash()

	s, err := l.sessions.CreateSession(ctx, userID, hash[:], time.Now().Add(l.sessionTTL))
	if err != nil {
		return IssuedSession{}, err
	}

	return IssuedSession{Token: token.String(), ExpiresAt: s.ExpiresAt}, nil
}
