package profile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestDeleteMe(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	deletionJobs := func(t *testing.T, id uuid.UUID) int {
		t.Helper()
		var n int
		err := e.pool.QueryRow(ctx,
			"SELECT count(*) FROM jobs WHERE kind = $1 AND payload->>'userId' = $2",
			DeleteAccountArgs{}.Kind(), id.String()).Scan(&n)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}

	t.Run("marks the student deleting, frees their login and queues the cleanup", func(t *testing.T) {
		u := e.otpUser(t)

		if rec := e.api.Delete("/profile/me", u.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}

		var deleting bool
		var email, usosID *string
		err := e.pool.QueryRow(ctx, "SELECT deleted_at IS NOT NULL, email, usos_user_id FROM users WHERE id = $1", u.id).
			Scan(&deleting, &email, &usosID)
		if err != nil {
			t.Fatal(err)
		}
		if !deleting || email != nil || usosID != nil {
			t.Errorf("got deleting=%v email=%v usos=%v, want deleting and no login", deleting, email, usosID)
		}
		if n := deletionJobs(t, u.id); n != 1 {
			t.Errorf("got %d deletion jobs, want 1", n)
		}
	})

	t.Run("the session stops working right away", func(t *testing.T) {
		u := e.otpUser(t)

		if rec := e.api.Delete("/profile/me", u.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}
		if rec := e.api.Get("/profile/me", u.bearer); rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 after deletion, got %d", rec.Code)
		}
	})

	t.Run("an old sign-in is a 403 and deletes nothing", func(t *testing.T) {
		u := e.otpUser(t)
		if _, err := e.pool.Exec(ctx, "UPDATE sessions SET created_at = NOW() - interval '1 hour' WHERE user_id = $1", u.id); err != nil {
			t.Fatal(err)
		}

		expectError(t, e.api.Delete("/profile/me", u.bearer), http.StatusForbidden, "reauthentication_required")

		var deleting bool
		if err := e.pool.QueryRow(ctx, "SELECT deleted_at IS NOT NULL FROM users WHERE id = $1", u.id).Scan(&deleting); err != nil {
			t.Fatal(err)
		}
		if deleting {
			t.Error("the account must not be marked deleting")
		}
		if n := deletionJobs(t, u.id); n != 0 {
			t.Errorf("got %d deletion jobs, want 0", n)
		}
	})

	t.Run("a double tap queues one cleanup", func(t *testing.T) {
		u := e.otpUser(t)

		var wg sync.WaitGroup
		recs := make([]*httptest.ResponseRecorder, 2)
		for i := range recs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				recs[i] = e.api.Delete("/profile/me", u.bearer)
			}()
		}
		wg.Wait()

		for _, rec := range recs {
			if rec.Code != http.StatusNoContent && rec.Code != http.StatusUnauthorized {
				t.Errorf("expected 204 or 401, got %d: %s", rec.Code, rec.Body.String())
			}
		}
		if n := deletionJobs(t, u.id); n != 1 {
			t.Errorf("got %d deletion jobs, want 1", n)
		}
	})

	t.Run("summaries leave out a student being deleted", func(t *testing.T) {
		u := e.otpUser(t)
		if rec := e.api.Delete("/profile/me", u.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}

		summaries, err := e.module.Summaries(ctx, []uuid.UUID{u.id})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := summaries[u.id]; ok {
			t.Error("a student being deleted must not have a summary")
		}
	})

	t.Run("needs a session", func(t *testing.T) {
		if rec := e.api.Delete("/profile/me"); rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})
}
