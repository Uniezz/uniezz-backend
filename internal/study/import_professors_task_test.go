package study

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/google/uuid"
)

type fakeEmployee struct {
	externalID uuid.UUID
	first      string
	last       string
	degree     string
}

type fakeRadon struct {
	mu          sync.Mutex
	institution uuid.UUID
	employees   []fakeEmployee
	pageSize    int
	requests    int
}

func (f *fakeRadon) set(employees ...fakeEmployee) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.employees = employees
}

func (f *fakeRadon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	start := 0
	if token := r.URL.Query().Get("token"); token != "" {
		start, _ = strconv.Atoi(token)
	}
	results := []map[string]any{}
	if r.URL.Query().Get("employingInstitutionUuid") == f.institution.String() {
		for i := start; i < len(f.employees) && i < start+f.pageSize; i++ {
			emp := f.employees[i]
			results = append(results, map[string]any{
				"id":                 "R-" + emp.externalID.String(),
				"calculatedEduLevel": emp.degree,
				"personalData":       map[string]any{"firstName": emp.first, "lastName": emp.last},
				"employeeInInstitutionUuids": []map[string]any{
					{"employeeInInstitutionUuid": uuid.NewString(), "employingInstitutionUuid": uuid.NewString()},
					{"employeeInInstitutionUuid": emp.externalID, "employingInstitutionUuid": f.institution},
				},
			})
		}
	}
	next := any(nil)
	if start+f.pageSize < len(f.employees) {
		next = strconv.Itoa(start + f.pageSize)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"results":    results,
		"pagination": map[string]any{"maxCount": len(f.employees), "token": next},
	})
}

type importFixture struct {
	e           *testEnv
	radon       *fakeRadon
	client      *radonClient
	institution radonInstitution
}

func newImportFixture(t *testing.T, e *testEnv) *importFixture {
	t.Helper()
	institution := radonInstitution{university: auth.UniversityID("t-" + uuid.NewString()[:8]), id: uuid.New()}
	radon := &fakeRadon{institution: institution.id, pageSize: 2}
	srv := httptest.NewServer(radon)
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM professors WHERE university_id = $1", string(institution.university))
	})
	return &importFixture{e: e, radon: radon, client: newRadonClient(srv.URL), institution: institution}
}

func (f *importFixture) sync(t *testing.T) error {
	t.Helper()
	return f.e.module.syncUniversity(context.Background(), f.client, f.institution)
}

func (f *importFixture) professorID(t *testing.T, externalID uuid.UUID) (uuid.UUID, bool) {
	t.Helper()
	var id uuid.UUID
	var active bool
	err := f.e.pool.QueryRow(context.Background(), "SELECT id, active FROM professors WHERE external_id = $1", externalID).Scan(&id, &active)
	if err != nil {
		t.Fatalf("professor %s: %v", externalID, err)
	}
	return id, active
}

func people(n int) []fakeEmployee {
	out := make([]fakeEmployee, n)
	for i := range out {
		out[i] = fakeEmployee{externalID: uuid.New(), first: "Person", last: strconv.Itoa(i), degree: "dr"}
	}
	return out
}

func TestSyncUniversity(t *testing.T) {
	e := newTestEnv(t)

	t.Run("imports every page with clean names and keeps ratings on the next run", func(t *testing.T) {
		f := newImportFixture(t, e)
		staff := people(4)
		staff[0] = fakeEmployee{externalID: uuid.New(), first: "<b>Sławomir</b>", last: "Kotyra", degree: "dr hab."}
		f.radon.set(staff...)
		if err := f.sync(t); err != nil {
			t.Fatal(err)
		}
		if f.radon.requests != 2 {
			t.Errorf("requests = %d, want 2 pages", f.radon.requests)
		}

		id, _ := f.professorID(t, staff[0].externalID)
		viewer := e.student(t)
		got := decode[Professor](t, e.api.Get("/study/professors/"+id.String(), viewer.bearer), http.StatusOK)
		if got.Name != "Sławomir Kotyra" || got.Degree == nil || *got.Degree != "dr hab." {
			t.Fatalf("professor = %#v", got)
		}
		decode[Professor](t, e.api.Put("/study/professors/"+id.String()+"/rating", viewer.bearer, map[string]any{"rating": 5}), http.StatusOK)

		if err := f.sync(t); err != nil {
			t.Fatal(err)
		}
		again := decode[Professor](t, e.api.Get("/study/professors/"+id.String(), viewer.bearer), http.StatusOK)
		if again.RatingCount != 1 || again.RatingAverage != 5 {
			t.Fatalf("import wiped the rating: %#v", again)
		}
	})

	t.Run("retires a professor who left and refuses to rate them", func(t *testing.T) {
		f := newImportFixture(t, e)
		staff := people(4)
		f.radon.set(staff...)
		if err := f.sync(t); err != nil {
			t.Fatal(err)
		}
		f.radon.set(staff[1:]...)
		if err := f.sync(t); err != nil {
			t.Fatal(err)
		}
		id, active := f.professorID(t, staff[0].externalID)
		if active {
			t.Fatal("the professor who left is still active")
		}
		viewer := e.student(t)
		expectError(t, e.api.Get("/study/professors/"+id.String(), viewer.bearer), http.StatusNotFound, "not_found")
		expectError(t, e.api.Put("/study/professors/"+id.String()+"/rating", viewer.bearer, map[string]any{"rating": 3}), http.StatusNotFound, "not_found")
	})

	t.Run("refuses to retire more than 30 percent at once", func(t *testing.T) {
		f := newImportFixture(t, e)
		staff := people(4)
		f.radon.set(staff...)
		if err := f.sync(t); err != nil {
			t.Fatal(err)
		}
		f.radon.set(staff[:2]...)
		if err := f.sync(t); err == nil {
			t.Fatal("expected the guard to stop the import")
		}
		for _, person := range staff {
			if _, active := f.professorID(t, person.externalID); !active {
				t.Errorf("professor %s was retired", person.externalID)
			}
		}
	})

	t.Run("an empty answer is an error and changes nothing", func(t *testing.T) {
		f := newImportFixture(t, e)
		staff := people(2)
		f.radon.set(staff...)
		if err := f.sync(t); err != nil {
			t.Fatal(err)
		}
		f.radon.set()
		if err := f.sync(t); err == nil {
			t.Fatal("expected an error for zero employees")
		}
		if _, active := f.professorID(t, staff[0].externalID); !active {
			t.Error("an empty answer retired professors")
		}
	})
}

func TestRadonClient(t *testing.T) {
	t.Run("stops with an error when pages never end", func(t *testing.T) {
		requests := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results":    []map[string]any{{"id": "x"}},
				"pagination": map[string]any{"maxCount": 1 << 30, "token": strconv.Itoa(requests)},
			})
		}))
		t.Cleanup(srv.Close)
		if _, err := newRadonClient(srv.URL).employees(context.Background(), uuid.New()); err == nil {
			t.Fatal("expected an error after the page cap")
		}
		if requests != radonMaxPages {
			t.Errorf("requests = %d, want %d", requests, radonMaxPages)
		}
	})

	t.Run("a status other than 200 is an error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		t.Cleanup(srv.Close)
		if _, err := newRadonClient(srv.URL).employees(context.Background(), uuid.New()); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestProfessorsAt(t *testing.T) {
	institution := radonInstitution{university: auth.UniversityUMCS, id: uuid.New()}
	post := uuid.New()
	employee := func(id, first, last string, posts ...uuid.UUID) radonEmployee {
		var e radonEmployee
		e.ID = id
		e.PersonalData.FirstName = first
		e.PersonalData.LastName = last
		for _, p := range posts {
			e.Posts = append(e.Posts, struct {
				EmployeeID    uuid.UUID `json:"employeeInInstitutionUuid"`
				InstitutionID uuid.UUID `json:"employingInstitutionUuid"`
			}{EmployeeID: p, InstitutionID: institution.id})
		}
		return e
	}

	got := professorsAt([]radonEmployee{
		employee("1", "Anna", "Nowak", post),
		employee("1", "Anna", "Nowak", post),
		employee("2", "Elsewhere", "Nowak"),
		employee("3", "<script></script>", "<b></b>", uuid.New()),
	}, institution)
	if len(got) != 1 || got[0].ExternalID != post || got[0].Name != "Anna Nowak" {
		t.Fatalf("professors = %#v", got)
	}
}
