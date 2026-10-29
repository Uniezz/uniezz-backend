package profile

import (
	"net/http"
	"testing"
)

func TestProgrammes(t *testing.T) {
	e := newTestEnv(t)

	create := func(t *testing.T, u testUser, body map[string]any) Programme {
		t.Helper()
		return decode[Programme](t, e.api.Post("/profile/programmes", u.bearer, body), http.StatusCreated)
	}
	list := func(t *testing.T, u testUser) []Programme {
		t.Helper()
		return decode[struct{ Programmes []Programme }](t, e.api.Get("/profile/programmes", u.bearer), http.StatusOK).Programmes
	}

	t.Run("creates with defaults and keeps a single primary", func(t *testing.T) {
		u := e.otpUser(t)

		first := create(t, u, map[string]any{"faculty": "Wydział Prawa", "programme": "Prawo", "isPrimary": true})
		if first.Status != "active" || first.Level != nil || first.YearOfStudy != nil || !first.IsPrimary {
			t.Errorf("unexpected defaults %+v", first)
		}

		second := create(t, u, map[string]any{
			"faculty": "WMFiI", "programme": "Informatyka", "level": "first_cycle",
			"modeOfStudies": "full_time", "yearOfStudy": 2, "isPrimary": true,
		})

		got := list(t, u)
		if len(got) != 2 || got[0].ID != second.ID || !got[0].IsPrimary || got[1].IsPrimary {
			t.Errorf("expected only the second programme primary, got %+v", got)
		}
	})

	t.Run("replace overwrites every field", func(t *testing.T) {
		u := e.otpUser(t)
		p := create(t, u, map[string]any{"faculty": "A", "programme": "B", "yearOfStudy": 3, "level": "second_cycle"})

		rec := e.api.Put("/profile/programmes/"+p.ID.String(), u.bearer, map[string]any{"faculty": "A", "programme": "C", "status": "graduated"})
		got := decode[Programme](t, rec, http.StatusOK)

		if got.Programme != "C" || got.Status != "graduated" || got.YearOfStudy != nil || got.Level != nil {
			t.Errorf("unexpected programme after replace %+v", got)
		}
	})

	t.Run("validates input", func(t *testing.T) {
		u := e.otpUser(t)

		for name, body := range map[string]map[string]any{
			"blank faculty":     {"faculty": "  ", "programme": "B"},
			"year out of range": {"faculty": "A", "programme": "B", "yearOfStudy": 7},
			"unknown level":     {"faculty": "A", "programme": "B", "level": "bachelor"},
			"missing programme": {"faculty": "A"},
		} {
			if rec := e.api.Post("/profile/programmes", u.bearer, body); rec.Code != http.StatusBadRequest {
				t.Errorf("%s: expected 400, got %d: %s", name, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("someone else's programme is not found", func(t *testing.T) {
		owner := e.otpUser(t)
		intruder := e.otpUser(t)
		p := create(t, owner, map[string]any{"faculty": "A", "programme": "B"})

		expectError(t, e.api.Put("/profile/programmes/"+p.ID.String(), intruder.bearer, map[string]any{"faculty": "X", "programme": "Y"}),
			http.StatusNotFound, "not_found")
		expectError(t, e.api.Delete("/profile/programmes/"+p.ID.String(), intruder.bearer), http.StatusNotFound, "not_found")

		if got := list(t, owner); len(got) != 1 || got[0].Programme != "B" {
			t.Errorf("owner's programme must be untouched, got %+v", got)
		}
	})

	t.Run("deletes", func(t *testing.T) {
		u := e.otpUser(t)
		p := create(t, u, map[string]any{"faculty": "A", "programme": "B"})

		if rec := e.api.Delete("/profile/programmes/"+p.ID.String(), u.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := list(t, u); len(got) != 0 {
			t.Errorf("expected no programmes, got %+v", got)
		}
	})
}
