package moderation

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/google/uuid"
)

func TestListQueueHandler(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	insert := func(t *testing.T, reporter, author testUser, createdAt time.Time) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		err := e.pool.QueryRow(ctx, `
			INSERT INTO reports (reporter_id, target_type, target_id, target_author_id, reason, details, snapshot, created_at)
			VALUES ($1, 'comment', $2, $3, 'harassment', 'rude', '{"content": "reported comment"}', $4) RETURNING id`,
			reporter.id, uuid.New(), author.id, createdAt).Scan(&id)
		if err != nil {
			t.Fatalf("insert report: %v", err)
		}
		t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), "DELETE FROM reports WHERE id = $1", id) })
		return id
	}

	// walk pages through the whole queue and keeps only the given reports,
	// so rows left by other tests do not matter.
	walk := func(t *testing.T, viewer testUser, limit int, own ...uuid.UUID) []map[string]any {
		t.Helper()
		wanted := map[string]bool{}
		for _, id := range own {
			wanted[id.String()] = true
		}
		var mine []map[string]any
		cursor := ""
		for {
			path := "/moderation/reports?limit=" + strconv.Itoa(limit)
			if cursor != "" {
				path += "&cursor=" + url.QueryEscape(cursor)
			}
			body := decode[map[string]any](t, e.api.Get(path, viewer.bearer), http.StatusOK)
			items, _ := body["items"].([]any)
			for _, raw := range items {
				item, _ := raw.(map[string]any)
				if id, _ := item["id"].(string); wanted[id] {
					mine = append(mine, item)
				}
			}
			next, _ := body["nextCursor"].(string)
			if next == "" {
				return mine
			}
			cursor = next
		}
	}

	t.Run("a moderator sees pending reports newest first, without the reporter", func(t *testing.T) {
		reporter, author, moderator := e.user(t), e.user(t), e.staff(t, auth.RoleModerator)
		now := time.Now()
		newer := insert(t, reporter, author, now)
		older := insert(t, reporter, author, now.Add(-time.Minute))

		items := walk(t, moderator, 1, newer, older)
		if len(items) != 2 || items[0]["id"] != newer.String() || items[1]["id"] != older.String() {
			t.Fatalf("expected %s then %s across pages, got %v", newer, older, items)
		}
		item := items[0]
		if item["targetType"] != "comment" || item["status"] != "pending" || item["targetAuthorId"] != author.id.String() {
			t.Errorf("unexpected item: %v", item)
		}
		if snapshot, _ := item["snapshot"].(map[string]any); snapshot["content"] != "reported comment" {
			t.Errorf("expected the snapshot, got %v", item["snapshot"])
		}
		if _, ok := item["reporterId"]; ok {
			t.Error("the reporter must not be exposed to moderators")
		}
	})

	t.Run("an admin sees the queue too", func(t *testing.T) {
		reporter, author, admin := e.user(t), e.user(t), e.staff(t, auth.RoleAdmin)
		id := insert(t, reporter, author, time.Now())

		if items := walk(t, admin, 50, id); len(items) != 1 {
			t.Errorf("expected report %s in the queue, got %v", id, items)
		}
	})

	t.Run("a student gets 403", func(t *testing.T) {
		expectError(t, e.api.Get("/moderation/reports", e.user(t).bearer), http.StatusForbidden, "forbidden")
	})

	t.Run("a student with a broken cursor still gets 403", func(t *testing.T) {
		expectError(t, e.api.Get("/moderation/reports?cursor=%21%21", e.user(t).bearer), http.StatusForbidden, "forbidden")
	})

	t.Run("a moderator with a broken cursor gets 400", func(t *testing.T) {
		moderator := e.staff(t, auth.RoleModerator)
		expectError(t, e.api.Get("/moderation/reports?cursor=%21%21", moderator.bearer), http.StatusBadRequest, "invalid_cursor")
	})

	t.Run("without a session the request is unauthorized", func(t *testing.T) {
		if rec := e.api.Get("/moderation/reports"); rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestUpdateReportHandler(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	insert := func(t *testing.T, targetID uuid.UUID, status ReportStatus) uuid.UUID {
		t.Helper()
		reporter := e.user(t)
		var id uuid.UUID
		err := e.pool.QueryRow(ctx, `
			INSERT INTO reports (reporter_id, target_type, target_id, reason, status)
			VALUES ($1, 'post', $2, 'spam', $3) RETURNING id`,
			reporter.id, targetID, status).Scan(&id)
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
	path := func(id uuid.UUID) string { return "/moderation/reports/" + id.String() }

	t.Run("resolving one report closes every pending report on the same target", func(t *testing.T) {
		moderator := e.staff(t, auth.RoleModerator)
		post, other := uuid.New(), uuid.New()
		first, second := insert(t, post, ReportPending), insert(t, post, ReportPending)
		dismissed := insert(t, post, ReportDismissed)
		elsewhere := insert(t, other, ReportPending)

		got := decode[Report](t, e.api.Patch(path(first), moderator.bearer, map[string]any{"status": "reviewed"}), http.StatusOK)
		if got.ID != first || got.Status != ReportReviewed {
			t.Errorf("report = %+v", got)
		}
		for id, want := range map[uuid.UUID]ReportStatus{first: ReportReviewed, second: ReportReviewed, dismissed: ReportDismissed, elsewhere: ReportPending} {
			if status := statusOf(t, id); status != want {
				t.Errorf("status of %s = %s, want %s", id, status, want)
			}
		}
	})

	t.Run("sending the current status again changes nothing", func(t *testing.T) {
		admin := e.staff(t, auth.RoleAdmin)
		id := insert(t, uuid.New(), ReportDismissed)

		got := decode[Report](t, e.api.Patch(path(id), admin.bearer, map[string]any{"status": "dismissed"}), http.StatusOK)
		if got.Status != ReportDismissed {
			t.Errorf("status = %s, want dismissed", got.Status)
		}
	})

	t.Run("a report cannot be set back to pending", func(t *testing.T) {
		moderator := e.staff(t, auth.RoleModerator)
		id := insert(t, uuid.New(), ReportReviewed)

		expectError(t, e.api.Patch(path(id), moderator.bearer, map[string]any{"status": "pending"}), http.StatusBadRequest, "invalid_input")
		if status := statusOf(t, id); status != ReportReviewed {
			t.Errorf("status = %s, want reviewed", status)
		}
	})

	t.Run("an unknown report is not found", func(t *testing.T) {
		moderator := e.staff(t, auth.RoleModerator)

		expectError(t, e.api.Patch(path(uuid.New()), moderator.bearer, map[string]any{"status": "reviewed"}), http.StatusNotFound, "not_found")
	})

	t.Run("students get 403 and the report stays pending", func(t *testing.T) {
		id := insert(t, uuid.New(), ReportPending)

		expectError(t, e.api.Patch(path(id), e.user(t).bearer, map[string]any{"status": "reviewed"}), http.StatusForbidden, "forbidden")
		if status := statusOf(t, id); status != ReportPending {
			t.Errorf("status = %s, want pending", status)
		}
	})

	t.Run("needs a session", func(t *testing.T) {
		if rec := e.api.Patch(path(uuid.New()), map[string]any{"status": "reviewed"}); rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}
