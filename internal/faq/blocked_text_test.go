package faq

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func (e *testEnv) entriesBy(t *testing.T, authorID uuid.UUID) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), "SELECT count(*) FROM faq_entries WHERE author_id = $1", authorID).Scan(&n); err != nil {
		t.Fatalf("count entries: %v", err)
	}
	return n
}

func TestBlockedText(t *testing.T) {
	e := newTestEnv(t)

	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{name: "a spaced-out question", body: map[string]any{"question": "Gdzie jest k u r w a dziekanat?", "answer": "W budynku A."}},
		{name: "a leetspeak answer", body: map[string]any{"question": "Where is the dean's office?", "answer": "In building A, sh1t."}},
	} {
		t.Run("a proposal with "+tc.name+" is rejected and not stored", func(t *testing.T) {
			u := e.user(t)
			expectError(t, e.api.Post("/faq/proposals", u.bearer, tc.body), http.StatusBadRequest, "content_not_allowed")
			if n := e.entriesBy(t, u.id); n != 0 {
				t.Errorf("stored %d entries, want 0", n)
			}
		})
	}

	t.Run("a publisher's entry with blocked text is rejected too", func(t *testing.T) {
		publisher := e.userEmail(t, e.publisher)
		body := map[string]any{"question": "Where is the dean's office?", "answer": "In building A, sh1t."}
		expectError(t, e.api.Post("/faq", publisher.bearer, body), http.StatusBadRequest, "content_not_allowed")
		if n := e.entriesBy(t, publisher.id); n != 0 {
			t.Errorf("stored %d entries, want 0", n)
		}
	})

	t.Run("ordinary proposals in several languages are accepted", func(t *testing.T) {
		u := e.user(t)
		for _, body := range []map[string]any{
			{"question": "Gdzie jest dziekanat?", "answer": "W budynku A, na pierwszym piętrze."},
			{"question": "Де знаходиться деканат?", "answer": "У корпусі А, на першому поверсі."},
			{"question": "¿Dónde está la secretaría?", "answer": "En el edificio A."},
		} {
			decode[Entry](t, e.api.Post("/faq/proposals", u.bearer, body), http.StatusCreated)
		}
	})
}
