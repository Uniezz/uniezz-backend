package moderation

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/google/uuid"
)

func TestListActionsHandler(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	type entry struct {
		actor  uuid.UUID
		action ActionType
		target uuid.UUID
	}
	// listed pages through the whole log and keeps only entries by the given
	// actors, so actions left by other tests do not matter.
	listed := func(t *testing.T, admin testUser, actors ...testUser) []entry {
		t.Helper()
		var mine []entry
		cursor := ""
		for {
			p := "/moderation/actions?limit=2"
			if cursor != "" {
				p += "&cursor=" + url.QueryEscape(cursor)
			}
			page := decode[struct {
				Items      []Action `json:"items"`
				NextCursor *string  `json:"nextCursor"`
			}](t, e.api.Get(p, admin.bearer), http.StatusOK)
			for _, item := range page.Items {
				for _, a := range actors {
					if item.ActorID != nil && *item.ActorID == a.id {
						mine = append(mine, entry{a.id, item.Type, item.TargetID})
					}
				}
			}
			if page.NextCursor == nil {
				return mine
			}
			cursor = *page.NextCursor
		}
	}
	noContent := func(t *testing.T, method, p string, caller testUser, body ...any) {
		t.Helper()
		args := append([]any{caller.bearer}, body...)
		if rec := e.api.Do(method, p, args...); rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
			t.Fatalf("%s %s: got %d: %s", method, p, rec.Code, rec.Body.String())
		}
	}

	t.Run("every staff action is listed newest first with who did it", func(t *testing.T) {
		admin, moderator, student, candidate := e.staff(t, auth.RoleAdmin), e.staff(t, auth.RoleModerator), e.user(t), e.user(t)
		post := uuid.New()
		var report uuid.UUID
		if err := e.pool.QueryRow(ctx, `
			INSERT INTO reports (reporter_id, target_type, target_id, reason) VALUES ($1, 'comment', $2, 'spam') RETURNING id`,
			e.user(t).id, uuid.New()).Scan(&report); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), "DELETE FROM reports WHERE id = $1", report) })

		noContent(t, http.MethodPatch, "/moderation/reports/"+report.String(), moderator, map[string]any{"status": "dismissed"})
		noContent(t, http.MethodDelete, "/moderation/posts/"+post.String(), moderator)
		noContent(t, http.MethodPut, "/moderation/users/"+student.id.String()+"/ban", moderator)
		noContent(t, http.MethodDelete, "/moderation/users/"+student.id.String()+"/ban", admin)
		noContent(t, http.MethodPut, "/moderation/moderators/"+candidate.id.String(), admin)
		noContent(t, http.MethodDelete, "/moderation/moderators/"+candidate.id.String(), admin)

		want := []entry{
			{admin.id, ActionRevokeModerator, candidate.id},
			{admin.id, ActionGrantModerator, candidate.id},
			{admin.id, ActionUnban, student.id},
			{moderator.id, ActionBan, student.id},
			{moderator.id, ActionRemovePost, post},
			{moderator.id, ActionDismissReport, report},
		}
		if got := listed(t, admin, admin, moderator); !slices.Equal(got, want) {
			t.Errorf("listed = %v\nwant %v", got, want)
		}
	})

	t.Run("a refused action is not listed", func(t *testing.T) {
		admin, moderator := e.staff(t, auth.RoleAdmin), e.staff(t, auth.RoleModerator)

		expectError(t, e.api.Put("/moderation/users/"+admin.id.String()+"/ban", moderator.bearer), http.StatusConflict, "target_is_staff")
		if got := listed(t, admin, moderator); len(got) != 0 {
			t.Errorf("listed = %v, want nothing", got)
		}
	})

	t.Run("a broken cursor is invalid", func(t *testing.T) {
		expectError(t, e.api.Get("/moderation/actions?cursor=%21%21", e.staff(t, auth.RoleAdmin).bearer), http.StatusBadRequest, "invalid_cursor")
	})

	t.Run("moderators and students get 403", func(t *testing.T) {
		for _, caller := range []testUser{e.staff(t, auth.RoleModerator), e.user(t)} {
			expectError(t, e.api.Get("/moderation/actions", caller.bearer), http.StatusForbidden, "forbidden")
		}
	})

	t.Run("needs a session", func(t *testing.T) {
		if rec := e.api.Get("/moderation/actions"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}

func TestPruneActions(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	actor := e.user(t)

	insert := func(t *testing.T, age time.Duration) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := e.pool.QueryRow(ctx, `
			INSERT INTO moderation_actions (actor_id, type, target_id, created_at) VALUES ($1, $2, $3, NOW() - $4::interval) RETURNING id`,
			actor.id, ActionBan, uuid.New(), age).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	old, recent := insert(t, actionRetention+24*time.Hour), insert(t, actionRetention-24*time.Hour)

	if err := e.module.pruneActions(ctx, PruneActionsArgs{}); err != nil {
		t.Fatalf("pruneActions: %v", err)
	}
	for id, want := range map[uuid.UUID]bool{old: false, recent: true} {
		var found bool
		if err := e.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM moderation_actions WHERE id = $1)", id).Scan(&found); err != nil {
			t.Fatal(err)
		}
		if found != want {
			t.Errorf("action %s kept = %v, want %v", id, found, want)
		}
	}
}
