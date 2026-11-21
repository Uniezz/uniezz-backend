package moderation

import (
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/google/uuid"
)

func TestModeratorHandlers(t *testing.T) {
	e := newTestEnv(t)

	path := func(u testUser) string { return "/moderation/moderators/" + u.id.String() }

	// listed pages through every moderator one at a time and keeps only the
	// given students, so moderators left by other tests do not matter.
	listed := func(t *testing.T, admin testUser, own ...testUser) []uuid.UUID {
		t.Helper()
		var mine []uuid.UUID
		cursor := ""
		for {
			p := "/moderation/moderators?limit=1"
			if cursor != "" {
				p += "&cursor=" + url.QueryEscape(cursor)
			}
			page := decode[struct {
				Items      []Moderator `json:"items"`
				NextCursor *string     `json:"nextCursor"`
			}](t, e.api.Get(p, admin.bearer), http.StatusOK)
			for _, item := range page.Items {
				for _, u := range own {
					if item.UserID == u.id {
						mine = append(mine, item.UserID)
					}
				}
			}
			if page.NextCursor == nil {
				return mine
			}
			cursor = *page.NextCursor
		}
	}

	t.Run("a granted student can moderate at once and is listed, newest first", func(t *testing.T) {
		admin, first, second := e.staff(t, auth.RoleAdmin), e.user(t), e.user(t)

		for _, u := range []testUser{first, second} {
			if rec := e.api.Put(path(u), admin.bearer); rec.Code != http.StatusNoContent {
				t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
			}
		}
		if rec := e.api.Get("/moderation/reports", first.bearer); rec.Code != http.StatusOK {
			t.Errorf("new moderator got %d on the queue", rec.Code)
		}
		if got := listed(t, admin, first, second); !slices.Equal(got, []uuid.UUID{second.id, first.id}) {
			t.Errorf("listed = %v, want [%s %s]", got, second.id, first.id)
		}
	})

	t.Run("granting again changes nothing", func(t *testing.T) {
		admin, student := e.staff(t, auth.RoleAdmin), e.user(t)

		for range 2 {
			if rec := e.api.Put(path(student), admin.bearer); rec.Code != http.StatusNoContent {
				t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
			}
		}
		if got := listed(t, admin, student); len(got) != 1 {
			t.Errorf("listed %d times, want once", len(got))
		}
	})

	t.Run("a revoked moderator loses access at once, and revoking again succeeds", func(t *testing.T) {
		admin, moderator := e.staff(t, auth.RoleAdmin), e.staff(t, auth.RoleModerator)

		for range 2 {
			if rec := e.api.Delete(path(moderator), admin.bearer); rec.Code != http.StatusNoContent {
				t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
			}
		}
		expectError(t, e.api.Get("/moderation/reports", moderator.bearer), http.StatusForbidden, "forbidden")
		if got := listed(t, admin, moderator); len(got) != 0 {
			t.Errorf("revoked moderator is still listed")
		}
	})

	t.Run("an unknown student cannot be made a moderator", func(t *testing.T) {
		expectError(t, e.api.Put("/moderation/moderators/"+uuid.NewString(), e.staff(t, auth.RoleAdmin).bearer), http.StatusNotFound, "not_found")
	})

	t.Run("a broken cursor is invalid", func(t *testing.T) {
		expectError(t, e.api.Get("/moderation/moderators?cursor=%21%21", e.staff(t, auth.RoleAdmin).bearer), http.StatusBadRequest, "invalid_cursor")
	})

	for _, tc := range []struct {
		name   string
		method string
	}{
		{"listing", http.MethodGet},
		{"granting", http.MethodPut},
		{"revoking", http.MethodDelete},
	} {
		target := func(t *testing.T) string {
			if tc.method == http.MethodGet {
				return "/moderation/moderators"
			}
			return path(e.user(t))
		}

		t.Run(tc.name+" is for admins only: moderators and students get 403", func(t *testing.T) {
			for _, caller := range []testUser{e.staff(t, auth.RoleModerator), e.user(t)} {
				expectError(t, e.api.Do(tc.method, target(t), caller.bearer), http.StatusForbidden, "forbidden")
			}
		})

		t.Run(tc.name+" needs a session", func(t *testing.T) {
			if rec := e.api.Do(tc.method, target(t)); rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", rec.Code)
			}
		})
	}

	t.Run("a moderator cannot make another student a moderator", func(t *testing.T) {
		student := e.user(t)

		e.api.Put(path(student), e.staff(t, auth.RoleModerator).bearer)
		expectError(t, e.api.Get("/moderation/reports", student.bearer), http.StatusForbidden, "forbidden")
	})
}
