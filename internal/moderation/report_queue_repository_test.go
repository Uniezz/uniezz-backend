package moderation

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestQueueRepositoryList(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	insert := func(t *testing.T, reporter testUser, status string, authorID uuid.UUID, createdAt time.Time) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		err := e.pool.QueryRow(ctx, `
			INSERT INTO reports (reporter_id, target_type, target_id, target_author_id, reason, snapshot, status, created_at)
			VALUES ($1, 'post', $2, $3, 'spam', '{"content": "queued"}', $4, $5) RETURNING id`,
			reporter.id, uuid.New(), authorID, status, createdAt).Scan(&id)
		if err != nil {
			t.Fatalf("insert report: %v", err)
		}
		t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), "DELETE FROM reports WHERE id = $1", id) })
		return id
	}

	// walk pages through the whole queue and keeps only the given reports,
	// so rows left by other tests do not matter.
	walk := func(t *testing.T, pageSize int, own ...uuid.UUID) []queueRow {
		t.Helper()
		var mine []queueRow
		filter := queueFilter{Limit: pageSize}
		for {
			rows, err := e.module.queue.list(ctx, filter)
			if err != nil {
				t.Fatalf("list failed: %v", err)
			}
			for _, row := range rows {
				if slices.Contains(own, row.ID) {
					mine = append(mine, row)
				}
			}
			if len(rows) < pageSize {
				return mine
			}
			last := rows[len(rows)-1]
			filter.After = &queueCursor{CreatedAt: last.CreatedAt, ID: last.ID}
		}
	}
	ids := func(rows []queueRow) []uuid.UUID {
		out := make([]uuid.UUID, 0, len(rows))
		for _, row := range rows {
			out = append(out, row.ID)
		}
		return out
	}

	t.Run("returns only pending reports, newest first, with the snapshot and author", func(t *testing.T) {
		reporter := e.user(t)
		base := time.Now().UTC().Truncate(time.Microsecond)
		newest := insert(t, reporter, "pending", reporter.id, base)
		reviewed := insert(t, reporter, "reviewed", reporter.id, base.Add(time.Minute))
		dismissed := insert(t, reporter, "dismissed", reporter.id, base.Add(2*time.Minute))
		oldest := insert(t, reporter, "pending", reporter.id, base.Add(-time.Hour))

		rows := walk(t, 50, newest, reviewed, dismissed, oldest)
		if got, want := ids(rows), []uuid.UUID{newest, oldest}; !slices.Equal(got, want) {
			t.Fatalf("expected %v, got %v", want, got)
		}
		if rows[0].Snapshot["content"] != "queued" {
			t.Errorf("expected the snapshot to be decoded, got %v", rows[0].Snapshot)
		}
		if rows[0].TargetAuthorID == nil || *rows[0].TargetAuthorID != reporter.id {
			t.Errorf("expected target author %s, got %v", reporter.id, rows[0].TargetAuthorID)
		}
	})

	t.Run("pages one by one without skipping or repeating reports with the same time", func(t *testing.T) {
		reporter := e.user(t)
		base := time.Now().UTC().Truncate(time.Microsecond)
		newest := insert(t, reporter, "pending", reporter.id, base)
		tieA := insert(t, reporter, "pending", reporter.id, base.Add(-time.Hour))
		tieB := insert(t, reporter, "pending", reporter.id, base.Add(-time.Hour))
		oldest := insert(t, reporter, "pending", reporter.id, base.Add(-2*time.Hour))

		// Postgres orders uuids byte by byte, so the higher one comes first.
		tieFirst, tieSecond := tieA, tieB
		if bytes.Compare(tieA[:], tieB[:]) < 0 {
			tieFirst, tieSecond = tieB, tieA
		}

		rows := walk(t, 1, newest, tieA, tieB, oldest)
		if got, want := ids(rows), []uuid.UUID{newest, tieFirst, tieSecond, oldest}; !slices.Equal(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("content without an author has a null author, not the zero uuid", func(t *testing.T) {
		reporter := e.user(t)
		id := insert(t, reporter, "pending", uuid.Nil, time.Now())

		rows := walk(t, 50, id)
		if len(rows) != 1 {
			t.Fatalf("expected report %s in the queue, got %v", id, rows)
		}
		if rows[0].TargetAuthorID != nil {
			t.Errorf("expected a null author, got %s", rows[0].TargetAuthorID)
		}
	})
}
