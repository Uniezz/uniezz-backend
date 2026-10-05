package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func deleteUserOnCleanup(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	})
}

func upsertTestUser(t *testing.T, pool *pgxpool.Pool, repo *UserRepository, id Identity) *User {
	t.Helper()

	u, err := repo.Upsert(context.Background(), id)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	deleteUserOnCleanup(t, pool, u.ID)

	return u
}

func testUsosUserID() string {
	return "test-" + uuid.NewString()
}

func testEmail() string {
	return "test-" + uuid.NewString() + "@student.kul.pl"
}

func strOrNil(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func TestUserRepository_Integration(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewUserRepository(pool)
	ctx := context.Background()

	t.Run("USOS first login creates user", func(t *testing.T) {
		usosID := testUsosUserID()

		u := upsertTestUser(t, pool, repo, Identity{
			UniversityID: UniversityUMCS,
			UsosUserID:   usosID,
			Email:        "jan@umcs.pl",
			FirstName:    "Jan",
			LastName:     "Kowalski",
		})

		if u.ID == uuid.Nil {
			t.Fatal("expected generated user ID")
		}
		if u.UniversityID != UniversityUMCS {
			t.Errorf("expected university %q, got %q", UniversityUMCS, u.UniversityID)
		}
		if strOrNil(u.UsosUserID) != usosID {
			t.Errorf("expected usos user id %q, got %q", usosID, strOrNil(u.UsosUserID))
		}
		if strOrNil(u.Email) != "jan@umcs.pl" {
			t.Errorf("expected email jan@umcs.pl, got %q", strOrNil(u.Email))
		}
		if strOrNil(u.FirstName) != "Jan" || strOrNil(u.LastName) != "Kowalski" {
			t.Errorf("expected Jan Kowalski, got %q %q", strOrNil(u.FirstName), strOrNil(u.LastName))
		}
		if u.LastLoginAt == nil {
			t.Error("expected last_login_at to be set")
		}
	})

	t.Run("USOS repeated login returns the same user and bumps last_login_at", func(t *testing.T) {
		id := Identity{UniversityID: UniversityUMCS, UsosUserID: testUsosUserID(), FirstName: "Jan"}

		first := upsertTestUser(t, pool, repo, id)
		second := upsertTestUser(t, pool, repo, id)

		if second.ID != first.ID {
			t.Fatalf("expected the same user %v, got %v", first.ID, second.ID)
		}
		if second.LastLoginAt == nil || second.LastLoginAt.Before(*first.LastLoginAt) {
			t.Errorf("expected last_login_at to move forward: first %v, second %v", first.LastLoginAt, second.LastLoginAt)
		}
	})

	t.Run("Repeated login keeps values the user edited", func(t *testing.T) {
		id := Identity{UniversityID: UniversityUMCS, UsosUserID: testUsosUserID(), FirstName: "Jan", LastName: "Kowalski"}
		first := upsertTestUser(t, pool, repo, id)

		if _, err := pool.Exec(ctx, "UPDATE users SET first_name = 'Janek' WHERE id = $1", first.ID); err != nil {
			t.Fatalf("failed to simulate profile edit: %v", err)
		}

		second := upsertTestUser(t, pool, repo, id)

		if strOrNil(second.FirstName) != "Janek" {
			t.Errorf("expected edited first name Janek to survive login, got %q", strOrNil(second.FirstName))
		}
		if strOrNil(second.LastName) != "Kowalski" {
			t.Errorf("expected last name Kowalski, got %q", strOrNil(second.LastName))
		}
	})

	t.Run("Repeated login fills values that were empty", func(t *testing.T) {
		usosID := testUsosUserID()
		first := upsertTestUser(t, pool, repo, Identity{UniversityID: UniversityUMCS, UsosUserID: usosID})

		if first.FirstName != nil || first.LastName != nil || first.Email != nil {
			t.Fatalf("expected empty fields stored as NULL, got %+v", first)
		}

		second := upsertTestUser(t, pool, repo, Identity{
			UniversityID: UniversityUMCS,
			UsosUserID:   usosID,
			Email:        "jan@umcs.pl",
			FirstName:    "Jan",
			LastName:     "Kowalski",
		})

		if strOrNil(second.FirstName) != "Jan" || strOrNil(second.LastName) != "Kowalski" || strOrNil(second.Email) != "jan@umcs.pl" {
			t.Errorf("expected empty fields to be filled, got %q %q %q",
				strOrNil(second.FirstName), strOrNil(second.LastName), strOrNil(second.Email))
		}
	})

	t.Run("Same USOS id at two universities gives two users", func(t *testing.T) {
		usosID := testUsosUserID()

		umcs := upsertTestUser(t, pool, repo, Identity{UniversityID: UniversityUMCS, UsosUserID: usosID})
		umlub := upsertTestUser(t, pool, repo, Identity{UniversityID: UniversityUMLub, UsosUserID: usosID})

		if umcs.ID == umlub.ID {
			t.Fatal("expected different users for different universities")
		}
	})

	t.Run("OTP login creates user keyed by email", func(t *testing.T) {
		email := testEmail()

		u := upsertTestUser(t, pool, repo, Identity{UniversityID: UniversityKUL, Email: email})

		if strOrNil(u.Email) != email {
			t.Errorf("expected email %q, got %q", email, strOrNil(u.Email))
		}
		if u.UsosUserID != nil {
			t.Errorf("expected no usos user id, got %q", *u.UsosUserID)
		}
		if u.FirstName != nil || u.LastName != nil {
			t.Errorf("expected no names for OTP user, got %q %q", strOrNil(u.FirstName), strOrNil(u.LastName))
		}
	})

	t.Run("OTP email match is case-insensitive", func(t *testing.T) {
		email := testEmail()

		first := upsertTestUser(t, pool, repo, Identity{UniversityID: UniversityKUL, Email: email})
		second := upsertTestUser(t, pool, repo, Identity{UniversityID: UniversityKUL, Email: strings.ToUpper(email)})

		if second.ID != first.ID {
			t.Fatalf("expected the same user for %q and its uppercase form", email)
		}
	})

	t.Run("Same email at two OTP universities gives two users", func(t *testing.T) {
		email := testEmail()

		kul := upsertTestUser(t, pool, repo, Identity{UniversityID: UniversityKUL, Email: email})
		up := upsertTestUser(t, pool, repo, Identity{UniversityID: UniversityUP, Email: email})

		if kul.ID == up.ID {
			t.Fatal("expected different users for different universities")
		}
	})
}
