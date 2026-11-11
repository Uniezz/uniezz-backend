package moderation

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestAnonymiseReports(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	report := func(t *testing.T, reporter testUser, authorID uuid.UUID) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		err := e.pool.QueryRow(ctx, `
			INSERT INTO reports (reporter_id, target_type, target_id, target_author_id, reason, snapshot)
			VALUES ($1, 'user', $2, $2, 'spam', '{"bio": "kept for review"}') RETURNING id`,
			reporter.id, authorID).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), "DELETE FROM reports WHERE id = $1", id) })
		return id
	}
	state := func(t *testing.T, id uuid.UUID) (forgotten bool, author *uuid.UUID, snapshot string) {
		t.Helper()
		err := e.pool.QueryRow(ctx,
			"SELECT target_forgotten_at IS NOT NULL, target_author_id, snapshot::text FROM reports WHERE id = $1", id).
			Scan(&forgotten, &author, &snapshot)
		if err != nil {
			t.Fatal(err)
		}
		return forgotten, author, snapshot
	}

	t.Run("marks reports about a deleted student and keeps the copy for now", func(t *testing.T) {
		reporter, gone := e.user(t), e.user(t)
		id := report(t, reporter, gone.id)
		if _, err := e.pool.Exec(ctx, "UPDATE users SET deleted_at = NOW(), email = NULL WHERE id = $1", gone.id); err != nil {
			t.Fatal(err)
		}

		if err := e.module.anonymiseReports(ctx, AnonymiseReportsArgs{}); err != nil {
			t.Fatalf("anonymiseReports failed: %v", err)
		}

		forgotten, author, snapshot := state(t, id)
		if !forgotten || author == nil || snapshot == "{}" {
			t.Errorf("got forgotten=%v author=%v snapshot=%s, want marked with the copy kept", forgotten, author, snapshot)
		}
	})

	t.Run("anonymises reports forgotten longer than the retention", func(t *testing.T) {
		reporter, gone := e.user(t), e.user(t)
		id := report(t, reporter, gone.id)
		if _, err := e.pool.Exec(ctx, "UPDATE reports SET target_forgotten_at = '2000-01-01' WHERE id = $1", id); err != nil {
			t.Fatal(err)
		}

		if err := e.module.anonymiseReports(ctx, AnonymiseReportsArgs{}); err != nil {
			t.Fatal(err)
		}

		_, author, snapshot := state(t, id)
		if author != nil || snapshot != "{}" {
			t.Errorf("got author=%v snapshot=%s, want both cleared", author, snapshot)
		}
	})

	t.Run("leaves reports about content without an author alone", func(t *testing.T) {
		reporter := e.user(t)
		id := report(t, reporter, uuid.Nil)

		if err := e.module.anonymiseReports(ctx, AnonymiseReportsArgs{}); err != nil {
			t.Fatal(err)
		}

		if forgotten, _, _ := state(t, id); forgotten {
			t.Error("a report without a target author must not be marked")
		}
	})

	t.Run("leaves reports about an active student alone", func(t *testing.T) {
		reporter, active := e.user(t), e.user(t)
		id := report(t, reporter, active.id)

		if err := e.module.anonymiseReports(ctx, AnonymiseReportsArgs{}); err != nil {
			t.Fatal(err)
		}

		if forgotten, _, _ := state(t, id); forgotten {
			t.Error("a report about an active student must not be marked")
		}
	})
}
