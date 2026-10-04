package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
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
	l := newLogins(users, sessions, time.Hour)
	ctx := context.Background()

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
