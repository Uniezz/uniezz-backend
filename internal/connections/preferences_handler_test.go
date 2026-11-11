package connections

import (
	"net/http/httptest"

	"github.com/google/uuid"
	"net/http"
	"testing"
	"time"
)

func TestSavePreferences(t *testing.T) {
	e := newTestEnv(t)
	today := time.Now().UTC()
	minor := today.AddDate(-17, 0, 0).Format(time.DateOnly)
	child := today.AddDate(-10, 0, 0).Format(time.DateOnly)
	future := today.AddDate(0, 0, 2).Format(time.DateOnly)

	t.Run("a new student has empty preferences", func(t *testing.T) {
		u := e.user(t)
		got := decode[Preferences](t, e.api.Get("/connections/me", u.bearer), http.StatusOK)
		if got.BirthDate != nil || got.Intent != nil || got.OpenTo == nil || len(got.OpenTo) != 0 {
			t.Errorf("preferences = %+v", got)
		}
	})

	t.Run("dating needs an adult birth date", func(t *testing.T) {
		u := e.user(t)
		expectError(t, e.api.Patch("/connections/me", u.bearer, map[string]any{"openTo": []string{"dating"}}), http.StatusBadRequest, "dating_requires_adult")
		expectError(t, e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": minor, "openTo": []string{"dating"}}), http.StatusBadRequest, "dating_requires_adult")

		decode[Preferences](t, e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": minor, "openTo": []string{"friends"}}), http.StatusOK)
		expectError(t, e.api.Patch("/connections/me", u.bearer, map[string]any{"openTo": []string{"friends", "dating"}}), http.StatusBadRequest, "dating_requires_adult")
	})

	t.Run("a birth date outside 16 to 100 years is a 400", func(t *testing.T) {
		u := e.user(t)
		expectError(t, e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": child}), http.StatusBadRequest, "invalid_input")
		expectError(t, e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": future}), http.StatusBadRequest, "invalid_input")
	})

	t.Run("saved values come back cleaned and a missing field keeps its value", func(t *testing.T) {
		u := e.user(t)
		got := decode[Preferences](t, e.api.Patch("/connections/me", u.bearer, map[string]any{
			"birthDate": "2000-01-01", "openTo": []string{"friends", "dating", "friends"}, "intent": " <b>Climbing</b> ",
		}), http.StatusOK)
		if got.BirthDate == nil || *got.BirthDate != "2000-01-01" || len(got.OpenTo) != 2 || got.Intent == nil || *got.Intent != "Climbing" {
			t.Fatalf("saved = %+v", got)
		}

		got = decode[Preferences](t, e.api.Patch("/connections/me", u.bearer, map[string]any{"intent": ""}), http.StatusOK)
		if got.Intent != nil || len(got.OpenTo) != 2 || got.BirthDate == nil {
			t.Errorf("after clearing the intent = %+v", got)
		}
		if again := decode[Preferences](t, e.api.Get("/connections/me", u.bearer), http.StatusOK); again.Intent != nil || len(again.OpenTo) != 2 {
			t.Errorf("read back = %+v", again)
		}
	})
}

func TestOperationsNeedASession(t *testing.T) {
	e := newTestEnv(t)
	id := uuid.NewString()
	requests := []struct {
		name string
		rec  *httptest.ResponseRecorder
	}{
		{"get preferences", e.api.Get("/connections/me")},
		{"save preferences", e.api.Patch("/connections/me", map[string]any{"openTo": []string{"friends"}})},
		{"discover", e.api.Get("/connections/discover?mode=friends")},
		{"act on a candidate", e.api.Put("/connections/candidates/"+id, map[string]any{"mode": "friends", "action": "like"})},
		{"list connections", e.api.Get("/connections")},
		{"delete a connection", e.api.Delete("/connections/" + id)},
		{"block", e.api.Put("/connections/blocks/" + id)},
		{"unblock", e.api.Delete("/connections/blocks/" + id)},
	}
	for _, tc := range requests {
		t.Run(tc.name, func(t *testing.T) {
			expectError(t, tc.rec, http.StatusUnauthorized, "unauthorized")
		})
	}
}

func TestAgeOn(t *testing.T) {
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		birth string
		want  int
	}{
		{"2008-03-01", 18},
		{"2008-03-02", 17},
		{"2008-02-29", 18},
		{"2000-02-29", 26},
	}
	for _, c := range cases {
		birth, _ := time.Parse(time.DateOnly, c.birth)
		if got := ageOn(birth, day); got != c.want {
			t.Errorf("ageOn(%s) = %d, want %d", c.birth, got, c.want)
		}
	}
}
