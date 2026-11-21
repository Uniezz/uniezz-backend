package moderation

import (
	"context"
	"net/http"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/google/uuid"
)

func TestRemovalHandlers(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	pendingReport := func(t *testing.T, targetType string, targetID uuid.UUID) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		err := e.pool.QueryRow(ctx, `
			INSERT INTO reports (reporter_id, target_type, target_id, reason) VALUES ($1, $2, $3, 'spam') RETURNING id`,
			e.user(t).id, targetType, targetID).Scan(&id)
		if err != nil {
			t.Fatalf("insert report: %v", err)
		}
		t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), "DELETE FROM reports WHERE id = $1", id) })
		return id
	}
	statusOf := func(t *testing.T, id uuid.UUID) ReportStatus {
		t.Helper()
		var status ReportStatus
		if err := e.pool.QueryRow(ctx, "SELECT status FROM reports WHERE id = $1", id).Scan(&status); err != nil {
			t.Fatalf("read status: %v", err)
		}
		return status
	}

	for _, tc := range []struct {
		name       string
		targetType string
		path       string
		role       auth.Role
	}{
		{"a moderator removes a post", "post", "/moderation/posts/", auth.RoleModerator},
		{"a moderator removes a comment", "comment", "/moderation/comments/", auth.RoleModerator},
		{"a moderator removes an exam file", "file", "/moderation/files/", auth.RoleModerator},
		{"an admin removes a course", "course", "/moderation/courses/", auth.RoleAdmin},
	} {
		t.Run(tc.name+" and its pending reports become reviewed", func(t *testing.T) {
			target := uuid.New()
			report := pendingReport(t, tc.targetType, target)

			if rec := e.api.Delete(tc.path+target.String(), e.staff(t, tc.role).bearer); rec.Code != http.StatusNoContent {
				t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
			}
			if got := e.targets.removed[target]; got != tc.targetType {
				t.Errorf("removed as %q, want %q", got, tc.targetType)
			}
			if status := statusOf(t, report); status != ReportReviewed {
				t.Errorf("report status = %s, want reviewed", status)
			}
		})

		t.Run(tc.name+": students get 403", func(t *testing.T) {
			target := uuid.New()

			expectError(t, e.api.Delete(tc.path+target.String(), e.user(t).bearer), http.StatusForbidden, "forbidden")
			if _, ok := e.targets.removed[target]; ok {
				t.Error("content was removed")
			}
		})

		t.Run(tc.name+": needs a session", func(t *testing.T) {
			if rec := e.api.Delete(tc.path + uuid.NewString()); rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", rec.Code)
			}
		})
	}

	t.Run("a moderator cannot remove a course", func(t *testing.T) {
		target := uuid.New()

		expectError(t, e.api.Delete("/moderation/courses/"+target.String(), e.staff(t, auth.RoleModerator).bearer), http.StatusForbidden, "forbidden")
		if _, ok := e.targets.removed[target]; ok {
			t.Error("course was removed")
		}
	})
}
