package study

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/google/uuid"
)

type fakeCourse struct {
	name    string
	level   string
	status  string
	faculty string
}

type fakeRadonCourses struct {
	mu          sync.Mutex
	institution uuid.UUID
	courses     []fakeCourse
}

func (f *fakeRadonCourses) set(courses ...fakeCourse) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.courses = courses
}

func (f *fakeRadonCourses) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	results := []map[string]any{}
	if r.URL.Path == "/courses" && r.URL.Query().Get("leadingInstitutionUuid") == f.institution.String() {
		for _, c := range f.courses {
			units := []map[string]any{}
			if c.faculty != "" {
				units = append(units, map[string]any{"organizationalUnitFullName": "Uczelnia Testowa; " + c.faculty})
			}
			results = append(results, map[string]any{
				"courseUuid":          uuid.NewString(),
				"courseName":          c.name,
				"levelCode":           c.level,
				"currentStatusCode":   c.status,
				"organizationalUnits": units,
			})
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"results":    results,
		"pagination": map[string]any{"maxCount": len(results), "token": nil},
	})
}

type programmeFixture struct {
	e           *testEnv
	radon       *fakeRadonCourses
	client      *radonClient
	institution radonInstitution
}

func newProgrammeFixture(t *testing.T, e *testEnv) *programmeFixture {
	t.Helper()
	institution := radonInstitution{university: auth.UniversityID(e.university), id: uuid.New()}
	radon := &fakeRadonCourses{institution: institution.id}
	srv := httptest.NewServer(radon)
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM programmes WHERE university_id = $1", e.university)
	})
	return &programmeFixture{e: e, radon: radon, client: newRadonClient(srv.URL), institution: institution}
}

func (f *programmeFixture) sync(t *testing.T) error {
	t.Helper()
	return f.e.module.syncProgrammes(context.Background(), f.client, f.institution)
}

type storedProgramme struct {
	id      uuid.UUID
	level   string
	faculty *uuid.UUID
	active  bool
}

func (f *programmeFixture) stored(t *testing.T) map[string]storedProgramme {
	t.Helper()
	rows, err := f.e.pool.Query(context.Background(),
		"SELECT id, name, level, faculty_id, active FROM programmes WHERE university_id = $1", f.e.university)
	if err != nil {
		t.Fatalf("read programmes: %v", err)
	}
	defer rows.Close()
	out := map[string]storedProgramme{}
	for rows.Next() {
		var p storedProgramme
		var name string
		if err := rows.Scan(&p.id, &name, &p.level, &p.faculty, &p.active); err != nil {
			t.Fatalf("scan programme: %v", err)
		}
		out[name+"/"+p.level] = p
	}
	return out
}

func (e *testEnv) facultyName(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var name string
	if err := e.pool.QueryRow(context.Background(), "SELECT name FROM university_faculties WHERE id = $1", id).Scan(&name); err != nil {
		t.Fatalf("faculty name: %v", err)
	}
	return name
}

func TestSyncProgrammes(t *testing.T) {
	t.Run("imports running and closing programmes once per level with their faculty", func(t *testing.T) {
		e := newTestEnv(t)
		f := newProgrammeFixture(t, e)
		faculty := e.facultyName(t, e.faculty)
		f.radon.set(
			fakeCourse{name: "Informatyka", level: "2", status: "3", faculty: faculty},
			fakeCourse{name: "Informatyka", level: "3", status: "3", faculty: strings.ReplaceAll(faculty, " ", " - ")},
			fakeCourse{name: "informatyka", level: "2", status: "3"},
			fakeCourse{name: "Archeologia  <b>stara</b>", level: "2", status: "1", faculty: "Wydział Nieznany"},
			fakeCourse{name: "Lekarski", level: "1", status: "3"},
			fakeCourse{name: "Zamknięty", level: "2", status: "2"},
			fakeCourse{name: "Doktorat", level: "4", status: "3"},
		)
		if err := f.sync(t); err != nil {
			t.Fatalf("sync: %v", err)
		}

		got := f.stored(t)
		if len(got) != 4 {
			t.Fatalf("programmes = %+v", got)
		}
		first, ok := got["Informatyka/first_cycle"]
		if !ok || first.faculty == nil || *first.faculty != e.faculty {
			t.Errorf("first cycle = %+v", first)
		}
		if p, ok := got["Informatyka/second_cycle"]; !ok || p.faculty == nil || *p.faculty != e.faculty {
			t.Errorf("second cycle = %+v", p)
		}
		if p, ok := got["Archeologia stara/first_cycle"]; !ok || p.faculty != nil {
			t.Errorf("closing programme = %+v", p)
		}
		if _, ok := got["Lekarski/long_cycle"]; !ok {
			t.Errorf("long cycle missing: %+v", got)
		}
	})

	t.Run("a second run keeps ids and retires programmes POL-on dropped", func(t *testing.T) {
		e := newTestEnv(t)
		f := newProgrammeFixture(t, e)
		courses := []fakeCourse{}
		for _, name := range []string{"Biologia", "Chemia", "Fizyka", "Geografia"} {
			courses = append(courses, fakeCourse{name: name, level: "2", status: "3"})
		}
		f.radon.set(courses...)
		if err := f.sync(t); err != nil {
			t.Fatalf("first sync: %v", err)
		}
		before := f.stored(t)

		f.radon.set(courses[1:]...)
		if err := f.sync(t); err != nil {
			t.Fatalf("second sync: %v", err)
		}
		after := f.stored(t)
		if after["Biologia/first_cycle"].active {
			t.Error("dropped programme is still active")
		}
		if after["Chemia/first_cycle"].id != before["Chemia/first_cycle"].id || !after["Chemia/first_cycle"].active {
			t.Errorf("kept programme changed: %+v -> %+v", before["Chemia/first_cycle"], after["Chemia/first_cycle"])
		}

		f.radon.set(courses...)
		if err := f.sync(t); err != nil {
			t.Fatalf("third sync: %v", err)
		}
		if back := f.stored(t)["Biologia/first_cycle"]; !back.active || back.id != before["Biologia/first_cycle"].id {
			t.Errorf("returning programme = %+v", back)
		}
	})

	t.Run("a run that would retire too many programmes changes nothing", func(t *testing.T) {
		e := newTestEnv(t)
		f := newProgrammeFixture(t, e)
		f.radon.set(
			fakeCourse{name: "Biologia", level: "2", status: "3"},
			fakeCourse{name: "Chemia", level: "2", status: "3"},
			fakeCourse{name: "Fizyka", level: "2", status: "3"},
		)
		if err := f.sync(t); err != nil {
			t.Fatalf("first sync: %v", err)
		}
		f.radon.set(fakeCourse{name: "Matematyka", level: "2", status: "3"})
		if err := f.sync(t); err == nil {
			t.Fatal("expected the sync to refuse")
		}
		got := f.stored(t)
		if len(got) != 3 || !got["Biologia/first_cycle"].active {
			t.Errorf("programmes = %+v", got)
		}
	})

	t.Run("an empty list is an error", func(t *testing.T) {
		e := newTestEnv(t)
		f := newProgrammeFixture(t, e)
		if err := f.sync(t); err == nil {
			t.Fatal("expected an error")
		}
	})
}
