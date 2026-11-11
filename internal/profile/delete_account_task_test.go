package profile

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestDeleteAccountTask(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	userExists := func(t *testing.T, id uuid.UUID) bool {
		t.Helper()
		var exists bool
		if err := e.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)", id).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		return exists
	}
	markDeleting := func(t *testing.T, id uuid.UUID) {
		t.Helper()
		if _, err := e.pool.Exec(ctx, "UPDATE users SET deleted_at = NOW(), email = NULL WHERE id = $1", id); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("asks feed, study and media to forget the student, then deletes the row", func(t *testing.T) {
		u := e.otpUser(t)
		avatar := insertMedia(ctx, e.pool, u.id, "ready")
		if _, err := e.pool.Exec(ctx, "UPDATE users SET avatar_media_id = $2 WHERE id = $1", u.id, avatar); err != nil {
			t.Fatal(err)
		}
		markDeleting(t, u.id)

		if err := e.module.deleteAccountTask(ctx, DeleteAccountArgs{UserID: u.id}); err != nil {
			t.Fatalf("deleteAccountTask failed: %v", err)
		}

		if !e.feed.forgotten(u.id) || !e.study.forgotten(u.id) {
			t.Error("feed and study must forget the student")
		}
		if got := mediaStatus(t, e.pool, avatar); got != "gone" {
			t.Errorf("avatar is %q, want gone", got)
		}
		if userExists(t, u.id) {
			t.Error("the user row must be deleted")
		}
	})

	t.Run("a failed step keeps the row so a retry can finish", func(t *testing.T) {
		u := e.otpUser(t)
		markDeleting(t, u.id)
		e.study.mu.Lock()
		e.study.failFor[u.id] = errors.New("study is down")
		e.study.mu.Unlock()

		if err := e.module.deleteAccountTask(ctx, DeleteAccountArgs{UserID: u.id}); err == nil {
			t.Fatal("expected an error")
		}
		if !userExists(t, u.id) {
			t.Fatal("the user row must stay until every step succeeds")
		}

		e.study.mu.Lock()
		delete(e.study.failFor, u.id)
		e.study.mu.Unlock()
		if err := e.module.deleteAccountTask(ctx, DeleteAccountArgs{UserID: u.id}); err != nil {
			t.Fatalf("retry failed: %v", err)
		}
		if userExists(t, u.id) {
			t.Error("the retry must delete the user row")
		}
	})

	t.Run("running again after the row is gone succeeds", func(t *testing.T) {
		u := e.otpUser(t)
		markDeleting(t, u.id)
		args := DeleteAccountArgs{UserID: u.id}

		if err := e.module.deleteAccountTask(ctx, args); err != nil {
			t.Fatal(err)
		}
		if err := e.module.deleteAccountTask(ctx, args); err != nil {
			t.Errorf("second run failed: %v", err)
		}
	})

	t.Run("does not delete a student who did not ask", func(t *testing.T) {
		u := e.otpUser(t)

		if err := e.module.deleteAccountTask(ctx, DeleteAccountArgs{UserID: u.id}); err != nil {
			t.Fatal(err)
		}
		if !userExists(t, u.id) {
			t.Error("a student who is not deleting must stay")
		}
		if e.feed.forgotten(u.id) || e.study.forgotten(u.id) {
			t.Error("feed and study must not forget a student who is not deleting")
		}
	})
}

func TestResumeDeletions(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	jobsFor := func(t *testing.T, id uuid.UUID) int {
		t.Helper()
		var n int
		err := e.pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE kind = $1 AND payload->>'userId' = $2",
			DeleteAccountArgs{}.Kind(), id.String()).Scan(&n)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}

	stuck := e.otpUser(t)
	recent := e.otpUser(t)
	if _, err := e.pool.Exec(ctx, "UPDATE users SET deleted_at = '2000-01-01', email = NULL WHERE id = $1", stuck.id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, "UPDATE users SET deleted_at = NOW(), email = NULL WHERE id = $1", recent.id); err != nil {
		t.Fatal(err)
	}

	if err := e.module.resumeDeletions(ctx, ResumeDeletionsArgs{}); err != nil {
		t.Fatalf("resumeDeletions failed: %v", err)
	}

	if n := jobsFor(t, stuck.id); n != 1 {
		t.Errorf("stuck deletion: got %d jobs, want 1", n)
	}
	if n := jobsFor(t, recent.id); n != 0 {
		t.Errorf("recent deletion: got %d jobs, want 0; its own job is still running", n)
	}

	var restarted bool
	if err := e.pool.QueryRow(ctx, "SELECT deleted_at > NOW() - interval '1 minute' FROM users WHERE id = $1", stuck.id).Scan(&restarted); err != nil {
		t.Fatal(err)
	}
	if !restarted {
		t.Error("a restarted deletion must move to the back of the queue")
	}
}
