package faq

import (
	"testing"
	"time"
)

type entryPage struct {
	Items      []Entry `json:"items"`
	NextCursor *string `json:"nextCursor"`
}

func TestFAQ(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	other := e.user(t)

	t.Run("published list is empty", func(t *testing.T) {
		body := decode[entryPage](t, e.api.Get("/faq", author.bearer), 200)
		if len(body.Items) != 0 || body.NextCursor != nil {
			t.Fatalf("got %#v", body)
		}
	})

	t.Run("requires a session", func(t *testing.T) {
		if rec := e.api.Get("/faq"); rec.Code != 401 {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
		if rec := e.api.Post("/faq/proposals", map[string]any{"question": "How do I sign in?", "answer": "Use USOS."}); rec.Code != 401 {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	var proposed Entry
	t.Run("a proposal stays out of the published list", func(t *testing.T) {
		proposed = decode[Entry](t, e.api.Post("/faq/proposals", author.bearer, map[string]any{
			"question": "<b>How do I reset it?</b>",
			"answer":   "Ask the dean's office.",
		}), 201)
		if proposed.Status != statusProposed || proposed.Question != "How do I reset it?" {
			t.Fatalf("got %#v", proposed)
		}

		published := decode[entryPage](t, e.api.Get("/faq", author.bearer), 200)
		if len(published.Items) != 0 {
			t.Fatalf("published list = %#v", published.Items)
		}

		mine := decode[entryPage](t, e.api.Get("/faq/proposals", author.bearer), 200)
		if len(mine.Items) != 1 || mine.Items[0].ID != proposed.ID {
			t.Fatalf("mine = %#v", mine.Items)
		}
		theirs := decode[entryPage](t, e.api.Get("/faq/proposals", other.bearer), 200)
		if len(theirs.Items) != 0 {
			t.Fatalf("other student saw %#v", theirs.Items)
		}
	})

	t.Run("rejects a question that is too short", func(t *testing.T) {
		expectError(t, e.api.Post("/faq/proposals", author.bearer, map[string]any{
			"question": "Hi",
			"answer":   "No.",
		}), 400, "invalid_input")
	})

	t.Run("published page hides proposals and rejected entries", func(t *testing.T) {
		now := time.Now().UTC()
		e.insert(t, author.id, statusPublished, now.Add(-2*time.Hour))
		e.insert(t, author.id, statusPublished, now.Add(-time.Hour))
		e.insert(t, author.id, "rejected", now)

		page := decode[entryPage](t, e.api.Get("/faq?limit=1", author.bearer), 200)
		if len(page.Items) != 1 || page.Items[0].Status != statusPublished || page.NextCursor == nil {
			t.Fatalf("first page = %#v", page)
		}
		next := decode[entryPage](t, e.api.Get("/faq?limit=1&cursor="+*page.NextCursor, author.bearer), 200)
		if len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID || next.NextCursor != nil {
			t.Fatalf("second page = %#v", next)
		}
	})
}
