package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

func TestStaffRoles(t *testing.T) {
	pool := setupTestDB(t)
	m := Module{roles: newRoleRepository(database.New(pool))}
	ctx := context.Background()

	count := func(t *testing.T, userID uuid.UUID) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM user_roles WHERE user_id = $1 AND role = $2", userID, RoleModerator).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	t.Run("granting twice leaves one role, and revoking twice succeeds", func(t *testing.T) {
		userID := createTestUser(t, pool)

		for range 2 {
			if err := m.GrantRole(ctx, userID, RoleModerator); err != nil {
				t.Fatalf("GrantRole: %v", err)
			}
		}
		if n := count(t, userID); n != 1 {
			t.Errorf("roles after grant = %d, want 1", n)
		}

		for range 2 {
			if err := m.RevokeRole(ctx, userID, RoleModerator); err != nil {
				t.Fatalf("RevokeRole: %v", err)
			}
		}
		if n := count(t, userID); n != 0 {
			t.Errorf("roles after revoke = %d, want 0", n)
		}
	})

	t.Run("an unknown user cannot get a role", func(t *testing.T) {
		if err := m.GrantRole(ctx, uuid.New(), RoleModerator); !errors.Is(err, ErrUserNotFound) {
			t.Errorf("GrantRole = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("a deleted account cannot get a role", func(t *testing.T) {
		userID := createTestUser(t, pool)
		if _, err := pool.Exec(ctx, "UPDATE users SET deleted_at = NOW() WHERE id = $1", userID); err != nil {
			t.Fatal(err)
		}

		if err := m.GrantRole(ctx, userID, RoleModerator); !errors.Is(err, ErrUserNotFound) {
			t.Errorf("GrantRole = %v, want ErrUserNotFound", err)
		}
		if n := count(t, userID); n != 0 {
			t.Errorf("roles = %d, want 0", n)
		}
	})
}
