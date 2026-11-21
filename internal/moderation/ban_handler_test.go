package moderation

import (
	"context"
	"net/http"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/google/uuid"
)

func TestBanHandlers(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	path := func(id uuid.UUID) string { return "/moderation/users/" + id.String() + "/ban" }
	expectNoContent := func(t *testing.T, method, p string, caller testUser) {
		t.Helper()
		if rec := e.api.Do(method, p, caller.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	t.Run("a banned student loses their session and reports about them become reviewed", func(t *testing.T) {
		moderator, student := e.staff(t, auth.RoleModerator), e.user(t)
		var reportID uuid.UUID
		if err := e.pool.QueryRow(ctx, `
			INSERT INTO reports (reporter_id, target_type, target_id, target_author_id, reason)
			VALUES ($1, 'user', $2, $2, 'harassment') RETURNING id`, e.user(t).id, student.id).Scan(&reportID); err != nil {
			t.Fatal(err)
		}

		for range 2 {
			expectNoContent(t, http.MethodPut, path(student.id), moderator)
		}

		got := decode[Ban](t, e.api.Get(path(student.id), moderator.bearer), http.StatusOK)
		if got.UserID != student.id || got.BannedAt == nil {
			t.Errorf("ban = %+v", got)
		}
		expectError(t, e.api.Post("/reports", student.bearer, report("post", uuid.New())), http.StatusUnauthorized, "unauthorized")
		var status ReportStatus
		if err := e.pool.QueryRow(ctx, "SELECT status FROM reports WHERE id = $1", reportID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != ReportReviewed {
			t.Errorf("report status = %s, want reviewed", status)
		}
	})

	t.Run("an admin lifts a ban, and lifting it again succeeds", func(t *testing.T) {
		moderator, admin, student := e.staff(t, auth.RoleModerator), e.staff(t, auth.RoleAdmin), e.user(t)
		expectNoContent(t, http.MethodPut, path(student.id), moderator)

		for range 2 {
			expectNoContent(t, http.MethodDelete, path(student.id), admin)
		}
		if got := decode[Ban](t, e.api.Get(path(student.id), admin.bearer), http.StatusOK); got.BannedAt != nil {
			t.Errorf("bannedAt = %v, want null", got.BannedAt)
		}
	})

	t.Run("moderators and admins cannot be banned", func(t *testing.T) {
		moderator := e.staff(t, auth.RoleModerator)
		for _, role := range []auth.Role{auth.RoleModerator, auth.RoleAdmin} {
			target := e.staff(t, role)

			expectError(t, e.api.Put(path(target.id), moderator.bearer), http.StatusConflict, "target_is_staff")
		}
		expectError(t, e.api.Put(path(moderator.id), moderator.bearer), http.StatusConflict, "target_is_staff")
	})

	t.Run("an unknown student is not found", func(t *testing.T) {
		moderator := e.staff(t, auth.RoleModerator)

		expectError(t, e.api.Put(path(uuid.New()), moderator.bearer), http.StatusNotFound, "not_found")
		expectError(t, e.api.Get(path(uuid.New()), moderator.bearer), http.StatusNotFound, "not_found")
	})

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method+": students get 403", func(t *testing.T) {
			target := e.user(t)

			expectError(t, e.api.Do(method, path(target.id), e.user(t).bearer), http.StatusForbidden, "forbidden")
			if got := decode[Ban](t, e.api.Get(path(target.id), e.staff(t, auth.RoleAdmin).bearer), http.StatusOK); got.BannedAt != nil {
				t.Error("student was banned")
			}
		})

		t.Run(method+": needs a session", func(t *testing.T) {
			if rec := e.api.Do(method, path(uuid.New())); rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", rec.Code)
			}
		})
	}
}
