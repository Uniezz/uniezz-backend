package auth

import (
	"context"
	"errors"
	"fmt"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRequireUniversity(t *testing.T) {
	testCases := []struct {
		name    string
		id      UniversityID
		method  AuthType
		wantErr error
	}{
		{name: "USOS university with USOS", id: UniversityUMCS, method: AuthTypeUSOS},
		{name: "OTP university with OTP", id: UniversityKUL, method: AuthTypeOTP},
		{name: "USOS university with OTP", id: UniversityUMCS, method: AuthTypeOTP, wantErr: ErrWrongAuthMethod},
		{name: "OTP university with USOS", id: UniversityKUL, method: AuthTypeUSOS, wantErr: ErrWrongAuthMethod},
		{name: "unknown university", id: "unknown-uni", method: AuthTypeOTP, wantErr: ErrUnknownUniversity},
		{name: "empty university", id: "", method: AuthTypeUSOS, wantErr: ErrUnknownUniversity},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := requireUniversity(tc.id, tc.method)

			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				if u.ID != tc.id {
					t.Errorf("expected university %q, got %q", tc.id, u.ID)
				}
				return
			}

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestLogins_Integration(t *testing.T) {
	pool := setupTestDB(t)
	users := NewUserRepository(pool)
	sessions := NewSessionRepository(pool)
	l := newLogins(database.New(pool), users, sessions, newLoginCodeRepository(pool), time.Hour)
	ctx := context.Background()

	signInTestUser := func(t *testing.T) *User {
		t.Helper()
		u, err := l.signIn(ctx, Identity{UniversityID: UniversityUMCS, UsosUserID: testUsosUserID()})
		if err != nil {
			t.Fatalf("signIn failed: %v", err)
		}
		deleteUserOnCleanup(t, pool, u.ID)
		return u
	}

	t.Run("a login code exchanges for a session of its user", func(t *testing.T) {
		u := signInTestUser(t)

		code, err := l.issueLoginCode(ctx, u.ID)
		if err != nil {
			t.Fatalf("issueLoginCode failed: %v", err)
		}

		issued, err := l.exchange(ctx, code)
		if err != nil {
			t.Fatalf("exchange failed: %v", err)
		}

		hash, err := HashTokenString(issued.Token)
		if err != nil {
			t.Fatalf("exchanged token is not a valid session token: %v", err)
		}
		s, err := sessions.GetActiveSessionByHash(ctx, hash[:])
		if err != nil {
			t.Fatalf("expected the exchanged token to resolve to an active session: %v", err)
		}
		if s.UserID != u.ID {
			t.Errorf("expected session for user %v, got %v", u.ID, s.UserID)
		}
		if issued.Token == code {
			t.Error("the session token must differ from the login code")
		}
	})

	t.Run("a login code exchanges only once", func(t *testing.T) {
		u := signInTestUser(t)

		code, err := l.issueLoginCode(ctx, u.ID)
		if err != nil {
			t.Fatalf("issueLoginCode failed: %v", err)
		}
		if _, err := l.exchange(ctx, code); err != nil {
			t.Fatalf("first exchange failed: %v", err)
		}

		_, err = l.exchange(ctx, code)
		if !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("expected ErrInvalidCode on replay, got: %v", err)
		}
	})

	t.Run("a session token is not accepted as a login code", func(t *testing.T) {
		u := signInTestUser(t)

		issued, err := l.issueSession(ctx, u.ID)
		if err != nil {
			t.Fatalf("issueSession failed: %v", err)
		}

		_, err = l.exchange(ctx, issued.Token)
		if !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("expected ErrInvalidCode, got: %v", err)
		}
	})

	t.Run("a malformed code is ErrInvalidCode and maps to 401 invalid_code", func(t *testing.T) {
		for _, code := range []string{"", "not-a-code!!", "dGVzdA"} {
			_, err := l.exchange(ctx, code)
			if !errors.Is(err, ErrInvalidCode) {
				t.Fatalf("exchange(%q): expected ErrInvalidCode, got: %v", code, err)
			}

			status, errCode := errorResponse(err)
			if status != http.StatusUnauthorized || errCode != "invalid_code" {
				t.Errorf("exchange(%q): expected 401 invalid_code, got %d %q", code, status, errCode)
			}
		}
	})

	t.Run("signIn normalizes the identity before storing it", func(t *testing.T) {
		email := testEmail()

		u, err := l.signIn(ctx, Identity{
			UniversityID: UniversityKUL,
			Email:        "  " + strings.ToUpper(email) + " ",
			FirstName:    " Jan ",
		})
		if err != nil {
			t.Fatalf("signIn failed: %v", err)
		}
		deleteUserOnCleanup(t, pool, u.ID)

		if strOrNil(u.Email) != email {
			t.Errorf("expected normalized email %q, got %q", email, strOrNil(u.Email))
		}
		if strOrNil(u.FirstName) != "Jan" {
			t.Errorf("expected trimmed first name Jan, got %q", strOrNil(u.FirstName))
		}
	})

	t.Run("signIn rejects an invalid identity without touching the database", func(t *testing.T) {
		email := testEmail()

		_, err := l.signIn(ctx, Identity{UniversityID: UniversityUMCS, Email: email})
		if !errors.Is(err, ErrInvalidIdentity) {
			t.Fatalf("expected ErrInvalidIdentity, got: %v", err)
		}

		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM users WHERE email = $1", email).Scan(&count); err != nil {
			t.Fatalf("failed to count users: %v", err)
		}
		if count != 0 {
			t.Errorf("expected no user to be created, found %d", count)
		}
	})

	t.Run("issueSession returns a token that resolves to the user's session", func(t *testing.T) {
		u, err := l.signIn(ctx, Identity{UniversityID: UniversityUMCS, UsosUserID: testUsosUserID()})
		if err != nil {
			t.Fatalf("signIn failed: %v", err)
		}
		deleteUserOnCleanup(t, pool, u.ID)

		before := time.Now()
		issued, err := l.issueSession(ctx, u.ID)
		if err != nil {
			t.Fatalf("issueSession failed: %v", err)
		}

		hash, err := HashTokenString(issued.Token)
		if err != nil {
			t.Fatalf("issued token is not a valid session token: %v", err)
		}

		s, err := sessions.GetActiveSessionByHash(ctx, hash[:])
		if err != nil {
			t.Fatalf("expected issued token to resolve to an active session: %v", err)
		}
		if s.UserID != u.ID {
			t.Errorf("expected session for user %v, got %v", u.ID, s.UserID)
		}
		if !s.ExpiresAt.Equal(issued.ExpiresAt) {
			t.Errorf("expected ExpiresAt to match the stored session: issued %v, stored %v", issued.ExpiresAt, s.ExpiresAt)
		}

		wantExpiry := before.Add(time.Hour)
		if d := issued.ExpiresAt.Sub(wantExpiry); d < -time.Second || d > time.Second {
			t.Errorf("expected ExpiresAt about %v, got %v", wantExpiry, issued.ExpiresAt)
		}
	})

	t.Run("issueSession gives a new token on every login", func(t *testing.T) {
		u, err := l.signIn(ctx, Identity{UniversityID: UniversityUMCS, UsosUserID: testUsosUserID()})
		if err != nil {
			t.Fatalf("signIn failed: %v", err)
		}
		deleteUserOnCleanup(t, pool, u.ID)

		first, err := l.issueSession(ctx, u.ID)
		if err != nil {
			t.Fatalf("issueSession failed: %v", err)
		}
		second, err := l.issueSession(ctx, u.ID)
		if err != nil {
			t.Fatalf("issueSession failed: %v", err)
		}

		if first.Token == second.Token {
			t.Error("expected different tokens for different sessions")
		}
	})
}

type recordingProfiles struct {
	db          database.DB
	initialized []uuid.UUID
	err         error
	afterCommit int
}

func (s *recordingProfiles) InitProfile(ctx context.Context, userID uuid.UUID, _ InitialProfile) (func(), error) {
	var n int
	if err := s.db.Conn(ctx).QueryRow(ctx, "SELECT count(*) FROM users WHERE id = $1", userID).Scan(&n); err != nil || n != 1 {
		return nil, fmt.Errorf("profiles must see the new user in its transaction: n=%d err=%v", n, err)
	}
	s.initialized = append(s.initialized, userID)
	return func() { s.afterCommit++ }, s.err
}

func TestLogins_InitializesOnlyNewAccounts(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()

	newLoginsWithProfiles := func(profiles ProfileInitializer) *logins {
		l := newLogins(database.New(pool), NewUserRepository(pool), NewSessionRepository(pool), newLoginCodeRepository(pool), time.Hour)
		l.profiles = profiles
		return l
	}

	t.Run("the first login initializes the profile, later logins do not", func(t *testing.T) {
		profiles := &recordingProfiles{db: database.New(pool)}
		l := newLoginsWithProfiles(profiles)
		id := Identity{UniversityID: UniversityUMCS, UsosUserID: testUsosUserID(), FirstName: "Jan"}

		first, err := l.signIn(ctx, id)
		if err != nil {
			t.Fatalf("first signIn failed: %v", err)
		}
		deleteUserOnCleanup(t, pool, first.ID)

		if _, err := l.signIn(ctx, id); err != nil {
			t.Fatalf("second signIn failed: %v", err)
		}

		if len(profiles.initialized) != 1 || profiles.initialized[0] != first.ID {
			t.Errorf("expected one initialization for %v, got %v", first.ID, profiles.initialized)
		}
		if profiles.afterCommit != 1 {
			t.Errorf("expected afterCommit to run once, ran %d times", profiles.afterCommit)
		}
	})

	t.Run("a failed profile initialization rolls back the new account", func(t *testing.T) {
		boom := errors.New("programme insert failed")
		profiles := &recordingProfiles{db: database.New(pool), err: boom}
		l := newLoginsWithProfiles(profiles)
		usosID := testUsosUserID()

		_, err := l.signIn(ctx, Identity{UniversityID: UniversityUMCS, UsosUserID: usosID})
		if !errors.Is(err, boom) {
			t.Fatalf("expected the initialization error, got %v", err)
		}

		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM users WHERE usos_user_id = $1", usosID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("expected no user after a failed initialization, got %d", n)
		}
		if profiles.afterCommit != 0 {
			t.Error("afterCommit must not run when the transaction rolls back")
		}
	})
}
