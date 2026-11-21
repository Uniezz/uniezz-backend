package faq

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func (e *testEnv) coins(t *testing.T, u testUser) int {
	t.Helper()
	var n int
	err := e.pool.QueryRow(context.Background(), "SELECT COALESCE((SELECT coins FROM coin_balances WHERE user_id = $1), 0)", u.id).Scan(&n)
	if err != nil {
		t.Fatalf("read coins: %v", err)
	}
	return n
}

func (e *testEnv) propose(t *testing.T, u testUser) Entry {
	t.Helper()
	return decode[Entry](t, e.api.Post("/faq/proposals", u.bearer, map[string]any{
		"question": "How do I get a student card " + uuid.NewString()[:8] + "?",
		"answer":   "Ask the dean's office.",
	}), http.StatusCreated)
}

func TestApproveProposal(t *testing.T) {
	e := newTestEnv(t)
	staff := e.user(t)
	if _, err := e.pool.Exec(context.Background(), "INSERT INTO user_roles (user_id, role) VALUES ($1, 'moderator')", staff.id); err != nil {
		t.Fatalf("grant moderator: %v", err)
	}
	approve := func(id uuid.UUID) string {
		return "/faq/proposals/" + id.String() + "/approval"
	}

	t.Run("a moderator sees every student's pending proposals", func(t *testing.T) {
		author := e.user(t)
		proposal := e.propose(t, author)
		found := false
		cursor := ""
		for range 100 {
			page := decode[entryPage](t, e.api.Get("/faq/proposals/pending?limit=50"+cursor, staff.bearer), http.StatusOK)
			for _, item := range page.Items {
				if item.Status != statusProposed {
					t.Fatalf("pending list has %#v", item)
				}
				found = found || item.ID == proposal.ID
			}
			if page.NextCursor == nil {
				break
			}
			cursor = "&cursor=" + *page.NextCursor
		}
		if !found {
			t.Error("the proposal is not in the pending list")
		}
	})

	t.Run("a student cannot list pending proposals or approve one", func(t *testing.T) {
		student := e.user(t)
		proposal := e.propose(t, e.user(t))
		expectError(t, e.api.Get("/faq/proposals/pending", student.bearer), http.StatusForbidden, "forbidden")
		expectError(t, e.api.Put(approve(proposal.ID), student.bearer), http.StatusForbidden, "forbidden")
	})

	t.Run("approving publishes the proposal and pays the author 5 coins once", func(t *testing.T) {
		author := e.user(t)
		proposal := e.propose(t, author)

		got := decode[Entry](t, e.api.Put(approve(proposal.ID), staff.bearer), http.StatusOK)
		if got.ID != proposal.ID || got.Status != statusPublished {
			t.Fatalf("approved = %#v", got)
		}
		decode[Entry](t, e.api.Put(approve(proposal.ID), staff.bearer), http.StatusOK)
		if c := e.coins(t, author); c != 5 {
			t.Errorf("author coins = %d, want 5", c)
		}
	})

	t.Run("two approvals at once pay once", func(t *testing.T) {
		author := e.user(t)
		proposal := e.propose(t, author)
		codes := make([]int, 5)
		var wg sync.WaitGroup
		for i := range codes {
			wg.Go(func() { codes[i] = e.api.Put(approve(proposal.ID), staff.bearer).Code })
		}
		wg.Wait()
		for _, code := range codes {
			if code != http.StatusOK {
				t.Fatalf("codes = %v", codes)
			}
		}
		if c := e.coins(t, author); c != 5 {
			t.Errorf("author coins = %d, want 5", c)
		}
	})

	t.Run("a moderator approving their own proposal earns nothing", func(t *testing.T) {
		proposal := e.propose(t, staff)
		decode[Entry](t, e.api.Put(approve(proposal.ID), staff.bearer), http.StatusOK)
		if c := e.coins(t, staff); c != 0 {
			t.Errorf("moderator coins = %d, want 0", c)
		}
	})

	t.Run("an unknown or rejected entry is 404", func(t *testing.T) {
		expectError(t, e.api.Put(approve(uuid.New()), staff.bearer), http.StatusNotFound, "not_found")

		author := e.user(t)
		proposal := e.propose(t, author)
		if _, err := e.pool.Exec(context.Background(), "UPDATE faq_entries SET status = 'rejected' WHERE id = $1", proposal.ID); err != nil {
			t.Fatalf("reject: %v", err)
		}
		expectError(t, e.api.Put(approve(proposal.ID), staff.bearer), http.StatusNotFound, "not_found")
		if c := e.coins(t, author); c != 0 {
			t.Errorf("author coins = %d, want 0", c)
		}
	})

	t.Run("without a session both are 401", func(t *testing.T) {
		expectError(t, e.api.Get("/faq/proposals/pending"), http.StatusUnauthorized, "unauthorized")
		expectError(t, e.api.Put(approve(uuid.New())), http.StatusUnauthorized, "unauthorized")
	})
}
