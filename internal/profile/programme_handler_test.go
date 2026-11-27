package profile

import (
	"context"
	"net/http"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

func (e *testEnv) catalogueProgramme(t *testing.T, university, name, level string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := e.pool.QueryRow(context.Background(), `
		INSERT INTO programmes (university_id, faculty_id, name, level)
		VALUES ($1::text, (SELECT id FROM university_faculties WHERE university_id = $1::text ORDER BY position, id LIMIT 1), $2, $3)
		RETURNING id`, university, name, level).Scan(&id)
	if err != nil {
		t.Fatalf("insert catalogue programme: %v", err)
	}
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM programmes WHERE id = $1", id)
	})
	return id
}

func TestProgrammes(t *testing.T) {
	e := newTestEnv(t)

	create := func(t *testing.T, u testUser, body map[string]any) Programme {
		t.Helper()
		return decode[Programme](t, e.api.Post("/profile/programmes", u.bearer, body), http.StatusCreated)
	}
	pick := func(t *testing.T, name, level string, extra map[string]any) map[string]any {
		t.Helper()
		body := map[string]any{"programmeId": e.catalogueProgramme(t, "kul", name+" "+uuid.NewString()[:8], level)}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}
	list := func(t *testing.T, u testUser) []Programme {
		t.Helper()
		return decode[struct{ Programmes []Programme }](t, e.api.Get("/profile/programmes", u.bearer), http.StatusOK).Programmes
	}

	t.Run("creates with defaults and keeps a single primary", func(t *testing.T) {
		u := e.otpUser(t)

		first := create(t, u, pick(t, "Prawo", "long_cycle", map[string]any{"isPrimary": true}))
		if first.Status != "active" || first.ProgrammeID == nil || first.Level == nil || *first.Level != "long_cycle" ||
			first.Faculty == nil || first.YearOfStudy != nil || !first.IsPrimary {
			t.Errorf("unexpected defaults %+v", first)
		}

		second := create(t, u, pick(t, "Informatyka", "first_cycle", map[string]any{"modeOfStudies": "full_time", "yearOfStudy": 2, "isPrimary": true}))

		got := list(t, u)
		if len(got) != 2 || got[0].ID != second.ID || !got[0].IsPrimary || got[1].IsPrimary {
			t.Errorf("expected only the second programme primary, got %+v", got)
		}
	})

	t.Run("replace overwrites every field", func(t *testing.T) {
		u := e.otpUser(t)
		p := create(t, u, pick(t, "B", "second_cycle", map[string]any{"yearOfStudy": 3, "modeOfStudies": "part_time"}))

		body := pick(t, "C", "first_cycle", map[string]any{"status": "graduated"})
		rec := e.api.Put("/profile/programmes/"+p.ID.String(), u.bearer, body)
		got := decode[Programme](t, rec, http.StatusOK)

		if got.ProgrammeID == nil || *got.ProgrammeID != body["programmeId"] || got.Programme == p.Programme || got.Status != "graduated" ||
			got.YearOfStudy != nil || got.ModeOfStudies != nil || got.Level == nil || *got.Level != "first_cycle" {
			t.Errorf("unexpected programme after replace %+v", got)
		}
	})

	t.Run("validates input", func(t *testing.T) {
		u := e.otpUser(t)

		for name, body := range map[string]map[string]any{
			"year out of range":                 pick(t, "A", "first_cycle", map[string]any{"yearOfStudy": 7}),
			"unknown mode":                      pick(t, "A", "first_cycle", map[string]any{"modeOfStudies": "evening"}),
			"missing programme":                 {"yearOfStudy": 1},
			"a programme not in the catalogue":  {"programmeId": uuid.New()},
			"a programme of another university": {"programmeId": e.catalogueProgramme(t, "umcs", "Obcy "+uuid.NewString()[:8], "first_cycle")},
		} {
			if rec := e.api.Post("/profile/programmes", u.bearer, body); rec.Code != http.StatusBadRequest {
				t.Errorf("%s: expected 400, got %d: %s", name, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("someone else's programme is not found", func(t *testing.T) {
		owner := e.otpUser(t)
		intruder := e.otpUser(t)
		p := create(t, owner, pick(t, "B", "first_cycle", nil))

		expectError(t, e.api.Put("/profile/programmes/"+p.ID.String(), intruder.bearer, pick(t, "Y", "first_cycle", nil)),
			http.StatusNotFound, "not_found")
		expectError(t, e.api.Delete("/profile/programmes/"+p.ID.String(), intruder.bearer), http.StatusNotFound, "not_found")

		if got := list(t, owner); len(got) != 1 || got[0].Programme != p.Programme {
			t.Errorf("owner's programme must be untouched, got %+v", got)
		}
	})

	t.Run("deletes", func(t *testing.T) {
		u := e.otpUser(t)
		p := create(t, u, pick(t, "B", "first_cycle", nil))

		if rec := e.api.Delete("/profile/programmes/"+p.ID.String(), u.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := list(t, u); len(got) != 0 {
			t.Errorf("expected no programmes, got %+v", got)
		}
	})

	t.Run("deleting primary promotes the oldest remaining programme", func(t *testing.T) {
		u := e.otpUser(t)
		primary := create(t, u, pick(t, "Primary", "first_cycle", map[string]any{"isPrimary": true}))
		oldest := create(t, u, pick(t, "Oldest", "first_cycle", nil))
		newer := create(t, u, pick(t, "Newer", "first_cycle", nil))

		_, err := e.pool.Exec(context.Background(), `
			UPDATE student_programmes
			SET created_at = CASE id
				WHEN $1 THEN NOW() - INTERVAL '2 days'
				WHEN $2 THEN NOW() - INTERVAL '1 day'
				ELSE created_at
			END
			WHERE id IN ($1, $2)`, oldest.ID, newer.ID)
		if err != nil {
			t.Fatalf("set programme creation times: %v", err)
		}

		if rec := e.api.Delete("/profile/programmes/"+primary.ID.String(), u.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}

		got := list(t, u)
		if len(got) != 2 || got[0].ID != oldest.ID || !got[0].IsPrimary || got[1].IsPrimary {
			t.Errorf("expected oldest remaining programme to be the only primary, got %+v", got)
		}
	})

	t.Run("deleting non-primary leaves primary unchanged", func(t *testing.T) {
		u := e.otpUser(t)
		primary := create(t, u, pick(t, "Primary", "first_cycle", map[string]any{"isPrimary": true}))
		other := create(t, u, pick(t, "Other", "first_cycle", nil))

		if rec := e.api.Delete("/profile/programmes/"+other.ID.String(), u.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}

		got := list(t, u)
		if len(got) != 1 || got[0].ID != primary.ID || !got[0].IsPrimary {
			t.Errorf("expected original primary to remain, got %+v", got)
		}
	})

	t.Run("deleting the only primary programme leaves none", func(t *testing.T) {
		u := e.otpUser(t)
		only := create(t, u, pick(t, "Only", "first_cycle", map[string]any{"isPrimary": true}))

		if rec := e.api.Delete("/profile/programmes/"+only.ID.String(), u.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}

		if got := list(t, u); len(got) != 0 {
			t.Errorf("expected no programmes, got %+v", got)
		}
	})

	t.Run("first programme becomes primary even when not requested", func(t *testing.T) {
		u := e.otpUser(t)
		first := create(t, u, pick(t, "First", "first_cycle", nil))
		if !first.IsPrimary {
			t.Errorf("expected the only programme to be primary, got %+v", first)
		}
	})

	t.Run("replace that demotes the primary promotes another programme", func(t *testing.T) {
		u := e.otpUser(t)
		primary := create(t, u, pick(t, "Primary", "first_cycle", map[string]any{"isPrimary": true}))
		other := create(t, u, pick(t, "Other", "first_cycle", nil))

		rec := e.api.Put("/profile/programmes/"+primary.ID.String(), u.bearer, pick(t, "Primary", "first_cycle", nil))
		got := decode[Programme](t, rec, http.StatusOK)
		if got.IsPrimary {
			t.Errorf("expected demoted programme to stay non-primary, got %+v", got)
		}

		programmes := list(t, u)
		if len(programmes) != 2 || programmes[0].ID != other.ID || !programmes[0].IsPrimary || programmes[1].IsPrimary {
			t.Errorf("expected the other programme to be the only primary, got %+v", programmes)
		}
	})

	t.Run("replace that demotes the only programme keeps it primary", func(t *testing.T) {
		u := e.otpUser(t)
		only := create(t, u, pick(t, "Only", "first_cycle", map[string]any{"isPrimary": true}))

		rec := e.api.Put("/profile/programmes/"+only.ID.String(), u.bearer, pick(t, "Only", "first_cycle", nil))
		got := decode[Programme](t, rec, http.StatusOK)
		if !got.IsPrimary {
			t.Errorf("expected the only programme to stay primary, got %+v", got)
		}
	})

	t.Run("a programme from USOS is linked to the catalogue programme with its name and level", func(t *testing.T) {
		u := e.newUser(t, "umcs", "profile-"+uuid.NewString()+"@umcs.pl")
		name := "Informatyka " + uuid.NewString()[:8]
		first := e.catalogueProgramme(t, "umcs", name, "first_cycle")
		e.catalogueProgramme(t, "umcs", name, "second_cycle")
		ctx := context.Background()

		err := database.New(e.pool).WithTx(ctx, func(ctx context.Context) error {
			_, err := e.module.InitProfile(ctx, u.id, auth.InitialProfile{Programmes: []auth.InitialProgramme{
				{Faculty: "Wydział Matematyki, Fizyki i Informatyki", Programme: name, Level: "first_cycle", IsPrimary: true},
				{Faculty: "Wydział Prawa i Administracji", Programme: "Prawo " + uuid.NewString()[:8]},
			}})
			return err
		})
		if err != nil {
			t.Fatalf("InitProfile failed: %v", err)
		}

		got := list(t, u)
		if len(got) != 2 || got[0].ProgrammeID == nil || *got[0].ProgrammeID != first || got[1].ProgrammeID != nil {
			t.Errorf("programmes = %+v, want the first linked to %s and the second unlinked", got, first)
		}
	})
}
