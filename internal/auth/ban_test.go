package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

func TestBan(t *testing.T) {
	pool := setupTestDB(t)
	db := database.New(pool)
	m := Module{roles: newRoleRepository(db), bans: newBanRepository(db)}
	sessions := NewSessionRepository(db)
	ctx := context.Background()

	session := func(t *testing.T, userID uuid.UUID) ([]byte, error) {
		t.Helper()
		hash := GenerateSessionToken().Hash()
		_, err := sessions.CreateSession(ctx, userID, hash[:], time.Now().Add(time.Hour))
		return hash[:], err
	}

	t.Run("a ban ends every session and blocks new ones until unbanned", func(t *testing.T) {
		userID := createTestUser(t, pool)
		hash, err := session(t, userID)
		if err != nil {
			t.Fatal(err)
		}

		for range 2 {
			if err := m.Ban(ctx, userID); err != nil {
				t.Fatalf("Ban: %v", err)
			}
		}
		if at, err := m.BannedAt(ctx, userID); err != nil || at == nil {
			t.Errorf("BannedAt = %v, %v", at, err)
		}
		if _, err := sessions.GetActiveSessionByHash(ctx, hash); !errors.Is(err, ErrSessionNotFound) {
			t.Errorf("old session = %v, want ErrSessionNotFound", err)
		}
		if _, err := session(t, userID); !errors.Is(err, ErrBanned) {
			t.Errorf("new session = %v, want ErrBanned", err)
		}

		if err := m.Unban(ctx, userID); err != nil {
			t.Fatalf("Unban: %v", err)
		}
		if at, err := m.BannedAt(ctx, userID); err != nil || at != nil {
			t.Errorf("BannedAt after unban = %v, %v", at, err)
		}
		if _, err := session(t, userID); err != nil {
			t.Errorf("new session after unban: %v", err)
		}
	})

	t.Run("staff cannot be banned", func(t *testing.T) {
		userID := createTestUser(t, pool)
		if err := m.GrantRole(ctx, userID, RoleModerator); err != nil {
			t.Fatal(err)
		}

		if err := m.Ban(ctx, userID); !errors.Is(err, ErrStaff) {
			t.Errorf("Ban = %v, want ErrStaff", err)
		}
		if at, _ := m.BannedAt(ctx, userID); at != nil {
			t.Errorf("staff got banned at %v", at)
		}
	})

	t.Run("a banned student cannot be given a role", func(t *testing.T) {
		userID := createTestUser(t, pool)
		if err := m.Ban(ctx, userID); err != nil {
			t.Fatal(err)
		}

		if err := m.GrantRole(ctx, userID, RoleModerator); !errors.Is(err, ErrUserNotFound) {
			t.Errorf("GrantRole = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("an unknown user cannot be banned or read", func(t *testing.T) {
		if err := m.Ban(ctx, uuid.New()); !errors.Is(err, ErrUserNotFound) {
			t.Errorf("Ban = %v, want ErrUserNotFound", err)
		}
		if _, err := m.BannedAt(ctx, uuid.New()); !errors.Is(err, ErrUserNotFound) {
			t.Errorf("BannedAt = %v, want ErrUserNotFound", err)
		}
	})
}
