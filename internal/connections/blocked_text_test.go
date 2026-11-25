package connections

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func (e *testEnv) intentOf(t *testing.T, userID uuid.UUID) *string {
	t.Helper()
	var intent *string
	err := e.pool.QueryRow(context.Background(),
		"SELECT (SELECT intent FROM connection_profiles WHERE user_id = $1)", userID).Scan(&intent)
	if err != nil {
		t.Fatalf("read intent: %v", err)
	}
	return intent
}

func TestBlockedText(t *testing.T) {
	e := newTestEnv(t)

	t.Run("a blocked intent is rejected on the connections endpoint", func(t *testing.T) {
		u := e.user(t)
		expectError(t, e.api.Patch("/connections/me", u.bearer, map[string]any{"intent": "Szukam k u r w y"}), http.StatusBadRequest, "content_not_allowed")
		if intent := e.intentOf(t, u.id); intent != nil {
			t.Errorf("intent = %q, want none", *intent)
		}
	})

	t.Run("a blocked intent sent through the profile is a 400 and not a 500", func(t *testing.T) {
		u := e.user(t)
		expectError(t, e.api.Patch("/profile/me", u.bearer, map[string]any{"intent": "Looking for sh1t"}), http.StatusBadRequest, "content_not_allowed")
		if intent := e.intentOf(t, u.id); intent != nil {
			t.Errorf("intent = %q, want none", *intent)
		}
	})

	t.Run("an ordinary intent is accepted on both endpoints", func(t *testing.T) {
		u := e.user(t)
		rec := e.api.Patch("/connections/me", u.bearer, map[string]any{"intent": "Szukam partnera do wspinaczki"})
		if rec.Code != http.StatusOK {
			t.Fatalf("connections: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		rec = e.api.Patch("/profile/me", u.bearer, map[string]any{"intent": "Шукаю компанію на шахи"})
		if rec.Code != http.StatusOK {
			t.Fatalf("profile: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if intent := e.intentOf(t, u.id); intent == nil || *intent != "Шукаю компанію на шахи" {
			t.Errorf("intent = %v, want the Ukrainian one", intent)
		}
	})
}
