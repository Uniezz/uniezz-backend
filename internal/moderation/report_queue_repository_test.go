package moderation

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestIsModerator(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	grant := func(t *testing.T, u testUser) {
		t.Helper()
		if _, err := e.pool.Exec(ctx, "INSERT INTO moderators (user_id) VALUES ($1)", u.id); err != nil {
			t.Fatalf("grant moderator: %v", err)
		}
	}
	check := func(t *testing.T, u testUser) bool {
		t.Helper()
		ok, err := e.module.reports.isModerator(ctx, u.id)
		if err != nil {
			t.Fatalf("isModerator failed: %v", err)
		}
		return ok
	}

	t.Run("a student is not a moderator", func(t *testing.T) {
		if check(t, e.user(t)) {
			t.Error("expected a student without a grant not to be a moderator")
		}
	})

	t.Run("a granted user is a moderator", func(t *testing.T) {
		mod := e.user(t)
		grant(t, mod)
		if !check(t, mod) {
			t.Error("expected a granted user to be a moderator")
		}
	})

	t.Run("a deleted moderator loses access", func(t *testing.T) {
		mod := e.user(t)
		grant(t, mod)
		if _, err := e.pool.Exec(ctx, "UPDATE users SET deleted_at = NOW(), email = NULL WHERE id = $1", mod.id); err != nil {
			t.Fatal(err)
		}
		if check(t, mod) {
			t.Error("expected a soft-deleted moderator not to be a moderator")
		}
	})
}

func TestListPending(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	reporter := e.user(t)

	// Far in the future, so these rows come before any other pending report
	// in a shared database.
	base := time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC)
	insert := func(t *testing.T, status string, createdAt time.Time) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		err := e.pool.QueryRow(ctx, `
			INSERT INTO reports (reporter_id, target_type, target_id, target_author_id, reason, snapshot, status, created_at)
			VALUES ($1, 'post', $2, $1, 'spam', '{"content": "queued"}', $3, $4) RETURNING id`,
			reporter.id, uuid.New(), status, createdAt).Scan(&id)
		if err != nil {
			t.Fatalf("insert report: %v", err)
		}
		t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), "DELETE FROM reports WHERE id = $1", id) })
		return id
	}

	newest := insert(t, "pending", base)
	insert(t, "reviewed", base.Add(time.Minute))
	insert(t, "dismissed", base.Add(2*time.Minute))
	tieA := insert(t, "pending", base.Add(-time.Hour))
	tieB := insert(t, "pending", base.Add(-time.Hour))
	oldest := insert(t, "pending", base.Add(-2*time.Hour))

	// Postgres orders uuids byte by byte, so the higher one comes first.
	tieFirst, tieSecond := tieA, tieB
	if bytes.Compare(tieA[:], tieB[:]) < 0 {
		tieFirst, tieSecond = tieB, tieA
	}
	want := []uuid.UUID{newest, tieFirst, tieSecond, oldest}

	t.Run("returns only pending reports, newest first, with the snapshot", func(t *testing.T) {
		rows, err := e.module.reports.listPending(ctx, nil, len(want))
		if err != nil {
			t.Fatalf("listPending failed: %v", err)
		}
		if len(rows) != len(want) {
			t.Fatalf("expected %d rows, got %d", len(want), len(rows))
		}
		for i, row := range rows {
			if row.ID != want[i] {
				t.Errorf("row %d: expected %s, got %s", i, want[i], row.ID)
			}
			if row.Status != "pending" {
				t.Errorf("row %d: expected status pending, got %s", i, row.Status)
			}
		}
		if rows[0].Snapshot["content"] != "queued" {
			t.Errorf("expected the snapshot to be decoded, got %v", rows[0].Snapshot)
		}
		if rows[0].TargetAuthorID == nil || *rows[0].TargetAuthorID != reporter.id {
			t.Errorf("expected target author %s, got %v", reporter.id, rows[0].TargetAuthorID)
		}
	})

	t.Run("pages one by one without skipping or repeating ties", func(t *testing.T) {
		var after *queueCursor
		for i, wantID := range want {
			rows, err := e.module.reports.listPending(ctx, after, 1)
			if err != nil {
				t.Fatalf("page %d: listPending failed: %v", i, err)
			}
			if len(rows) != 1 || rows[0].ID != wantID {
				t.Fatalf("page %d: expected %s, got %v", i, wantID, rows)
			}
			after = &queueCursor{CreatedAt: rows[0].CreatedAt, ID: rows[0].ID}
		}
	})
}
