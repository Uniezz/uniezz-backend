package study

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
)

func (e *testEnv) programme(t *testing.T, name, level string, faculty *uuid.UUID, active bool) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := e.pool.QueryRow(context.Background(), `
		INSERT INTO programmes (university_id, faculty_id, name, level, active) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		e.university, faculty, name, level, active).Scan(&id)
	if err != nil {
		t.Fatalf("insert programme: %v", err)
	}
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM programmes WHERE id = $1", id)
	})
	return id
}

func TestListProgrammes(t *testing.T) {
	e := newTestEnv(t)

	list := func(t *testing.T, viewer student, query string) itemPage[CatalogueProgramme] {
		t.Helper()
		return decode[itemPage[CatalogueProgramme]](t, e.api.Get("/study/programmes?universityId="+e.university+query, viewer.bearer), http.StatusOK)
	}

	t.Run("requires a session", func(t *testing.T) {
		expectError(t, e.api.Get("/study/programmes?universityId="+e.university), http.StatusUnauthorized, "unauthorized")
	})

	t.Run("lists active programmes of the university by name with their faculty", func(t *testing.T) {
		viewer := e.student(t)
		faculty := e.newFaculty(t)
		marker := uuid.NewString()[:8]
		b := e.programme(t, "Bm "+marker, "first_cycle", &faculty, true)
		a := e.programme(t, "Am "+marker, "second_cycle", nil, true)
		e.programme(t, "Cm "+marker, "first_cycle", nil, false)

		got := list(t, viewer, "&q="+marker).Items
		if len(got) != 2 || got[0].ID != a || got[1].ID != b {
			t.Fatalf("programmes = %+v", got)
		}
		if got[0].FacultyID != nil || got[0].Faculty != nil || got[0].Level != ProgrammeSecondCycle {
			t.Errorf("without faculty = %+v", got[0])
		}
		if got[1].FacultyID == nil || *got[1].FacultyID != faculty || got[1].Faculty == nil || *got[1].Faculty != e.facultyName(t, faculty) {
			t.Errorf("with faculty = %+v", got[1])
		}
	})

	t.Run("filters by faculty and pages by name", func(t *testing.T) {
		viewer := e.student(t)
		faculty := e.newFaculty(t)
		for _, name := range []string{"Alfa", "Beta", "Gamma"} {
			e.programme(t, name, "first_cycle", &faculty, true)
		}
		e.programme(t, "Delta", "first_cycle", nil, true)

		query := "&facultyId=" + faculty.String() + "&limit=2"
		first := list(t, viewer, query)
		if len(first.Items) != 2 || first.NextCursor == nil || first.Items[0].Name != "Alfa" {
			t.Fatalf("first page = %+v", first)
		}
		second := list(t, viewer, query+"&cursor="+url.QueryEscape(*first.NextCursor))
		if len(second.Items) != 1 || second.NextCursor != nil || second.Items[0].Name != "Gamma" {
			t.Fatalf("second page = %+v", second)
		}
	})

	t.Run("a missing university is invalid input", func(t *testing.T) {
		viewer := e.student(t)
		expectError(t, e.api.Get("/study/programmes", viewer.bearer), http.StatusBadRequest, "invalid_input")
	})
}
