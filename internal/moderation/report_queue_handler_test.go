package moderation

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestListQueueHandler(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	reporter, author := e.user(t), e.user(t)
	moderator, student := e.user(t), e.user(t)
	if _, err := e.pool.Exec(ctx, "INSERT INTO moderators (user_id) VALUES ($1)", moderator.id); err != nil {
		t.Fatalf("grant moderator: %v", err)
	}

	// Far in the future, so it is the first item even in a shared database.
	var reportID uuid.UUID
	err := e.pool.QueryRow(ctx, `
		INSERT INTO reports (reporter_id, target_type, target_id, target_author_id, reason, details, snapshot, created_at)
		VALUES ($1, 'comment', $2, $3, 'harassment', 'rude', '{"content": "reported comment"}', $4) RETURNING id`,
		reporter.id, uuid.New(), author.id, time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC)).Scan(&reportID)
	if err != nil {
		t.Fatalf("insert report: %v", err)
	}
	t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), "DELETE FROM reports WHERE id = $1", reportID) })

	t.Run("a moderator sees the pending report first, without the reporter", func(t *testing.T) {
		rec := e.api.Get("/moderation/reports?limit=1", moderator.bearer)
		body := decode[map[string]any](t, rec, http.StatusOK)

		items, _ := body["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %v", body["items"])
		}
		item := items[0].(map[string]any)
		if item["id"] != reportID.String() {
			t.Errorf("expected report %s first, got %v", reportID, item["id"])
		}
		if item["targetType"] != "comment" || item["status"] != "pending" || item["targetAuthorId"] != author.id.String() {
			t.Errorf("unexpected item: %v", item)
		}
		if snapshot, _ := item["snapshot"].(map[string]any); snapshot["content"] != "reported comment" {
			t.Errorf("expected the snapshot, got %v", item["snapshot"])
		}
		if _, ok := item["reporterId"]; ok {
			t.Error("the reporter must not be exposed to moderators")
		}
		if _, ok := body["nextCursor"].(string); !ok {
			t.Errorf("expected a nextCursor with limit=1, got %v", body["nextCursor"])
		}
	})

	t.Run("a student gets 403", func(t *testing.T) {
		expectError(t, e.api.Get("/moderation/reports", student.bearer), http.StatusForbidden, "forbidden")
	})

	t.Run("a student with a broken cursor still gets 403", func(t *testing.T) {
		expectError(t, e.api.Get("/moderation/reports?cursor=%21%21", student.bearer), http.StatusForbidden, "forbidden")
	})

	t.Run("a moderator with a broken cursor gets 400", func(t *testing.T) {
		expectError(t, e.api.Get("/moderation/reports?cursor=%21%21", moderator.bearer), http.StatusBadRequest, "invalid_cursor")
	})

	t.Run("without a session the request is unauthorized", func(t *testing.T) {
		if rec := e.api.Get("/moderation/reports"); rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
