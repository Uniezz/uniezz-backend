package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

type universitiesEnv struct {
	pool   *pgxpool.Pool
	module Module
	api    humatest.TestAPI
}

func newUniversitiesEnv(t *testing.T) *universitiesEnv {
	t.Helper()
	pool := setupTestDB(t)
	m, err := New(pool, &config.Config{}, nil, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, api := humatest.New(t, httpapi.NewConfig(&config.Config{}))
	api.UseMiddleware(m.Guard(api))
	m.Register(api)
	return &universitiesEnv{pool: pool, module: m, api: api}
}

func (e *universitiesEnv) bearer(t *testing.T, roles ...Role) string {
	t.Helper()
	ctx := context.Background()
	userID := createTestUser(t, e.pool)
	for _, role := range roles {
		if err := e.module.GrantRole(ctx, userID, role); err != nil {
			t.Fatalf("GrantRole: %v", err)
		}
	}
	token := GenerateSessionToken()
	hash := token.Hash()
	if _, err := NewSessionRepository(database.New(e.pool)).CreateSession(ctx, userID, hash[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return "Authorization: Bearer " + token.String()
}

func (e *universitiesEnv) newID(t *testing.T) string {
	t.Helper()
	id := "t-" + uuid.NewString()[:8]
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM universities WHERE id = $1", id)
	})
	return id
}

func newDomain() string {
	return "d" + uuid.NewString()[:8] + ".example.pl"
}

func universityBody(id, domain string) map[string]any {
	return map[string]any{
		"id":         id,
		"name":       "Testowa Uczelnia",
		"authMethod": "OTP",
		"position":   100,
		"domains":    []string{domain},
		"faculties": []map[string]any{
			{"name": "Wydział Pierwszy", "address": "ul. Testowa 1", "latitude": 51.2, "longitude": 22.5},
			{"name": "Wydział Drugi", "address": "ul. Testowa 2", "latitude": 51.3, "longitude": 22.6},
		},
	}
}

func decodeUniversity[T any](t *testing.T, rec *httptest.ResponseRecorder, status int) T {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("expected %d, got %d: %s", status, rec.Code, rec.Body.String())
	}
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

func expectUniversityError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("expected %d, got %d: %s", status, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"`+code+`"`) {
		t.Errorf("expected code %q, got %s", code, rec.Body.String())
	}
}

func TestGetUniversities(t *testing.T) {
	e := newUniversitiesEnv(t)

	t.Run("lists the seeded universities in display order with domains and faculties", func(t *testing.T) {
		list := decodeUniversity[[]University](t, e.api.Get("/auth/universities"), http.StatusOK)
		byID := map[string]University{}
		for _, u := range list {
			byID[u.ID] = u
		}
		if len(list) < 2 || list[0].ID != "umcs" || list[1].ID != "umlub" {
			t.Fatalf("expected umcs and umlub first, got %v", list)
		}
		umcs := byID["umcs"]
		if umcs.AuthMethod != "USOS" || len(umcs.Faculties) != 12 || umcs.Domains == nil || len(umcs.Domains) != 0 {
			t.Errorf("umcs = %+v", umcs)
		}
		if umcs.RadonInstitutionID == nil || umcs.RadonInstitutionID.String() != "a013df22-d5c5-4871-849c-69e8c6e7e912" {
			t.Errorf("umcs radon id = %v", umcs.RadonInstitutionID)
		}
		if got := byID["wsei"].Domains; strings.Join(got, ",") != "studentwsei.pl,wsei.lublin.pl" {
			t.Errorf("wsei domains = %v", got)
		}
		if f := umcs.Faculties[2]; f.Name != "Wydział Chemii" || f.Location.Latitude != 51.24513 || f.Location.Longitude != 22.54247 {
			t.Errorf("umcs third faculty = %+v", f)
		}
	})

	t.Run("gets one university by id", func(t *testing.T) {
		kul := decodeUniversity[University](t, e.api.Get("/auth/universities/kul"), http.StatusOK)
		if kul.AuthMethod != "OTP" || strings.Join(kul.Domains, ",") != "student.kul.pl" {
			t.Errorf("kul = %+v", kul)
		}
	})

	t.Run("an unknown university is not found", func(t *testing.T) {
		expectUniversityError(t, e.api.Get("/auth/universities/nowhere"), http.StatusNotFound, "not_found")
	})
}

func TestCreateUniversity(t *testing.T) {
	e := newUniversitiesEnv(t)

	t.Run("requires a session", func(t *testing.T) {
		expectUniversityError(t, e.api.Post("/auth/universities", universityBody(e.newID(t), newDomain())), http.StatusUnauthorized, "unauthorized")
	})

	t.Run("a student cannot add a university", func(t *testing.T) {
		expectUniversityError(t, e.api.Post("/auth/universities", e.bearer(t), universityBody(e.newID(t), newDomain())), http.StatusForbidden, "forbidden")
	})

	t.Run("an admin adds a university whose students can log in right away", func(t *testing.T) {
		id, domain := e.newID(t), newDomain()
		created := decodeUniversity[University](t, e.api.Post("/auth/universities", e.bearer(t, RoleAdmin), universityBody(id, domain)), http.StatusCreated)
		if created.ID != id || created.Position != 100 || created.RadonInstitutionID != nil ||
			strings.Join(created.Domains, ",") != domain || len(created.Faculties) != 2 || created.Faculties[1].Name != "Wydział Drugi" {
			t.Errorf("created = %+v", created)
		}

		u, err := requireUniversity(context.Background(), e.module.universities, UniversityID(id), AuthTypeOTP)
		if err != nil {
			t.Fatalf("requireUniversity: %v", err)
		}
		if !u.acceptsEmail("jan@" + strings.ToUpper(domain)) {
			t.Error("expected the new domain to be accepted")
		}
	})

	t.Run("an id that exists is a conflict", func(t *testing.T) {
		admin := e.bearer(t, RoleAdmin)
		id := e.newID(t)
		decodeUniversity[University](t, e.api.Post("/auth/universities", admin, universityBody(id, newDomain())), http.StatusCreated)
		expectUniversityError(t, e.api.Post("/auth/universities", admin, universityBody(id, newDomain())), http.StatusConflict, "university_exists")
	})

	t.Run("a domain of another university is a conflict and adds nothing", func(t *testing.T) {
		id := e.newID(t)
		expectUniversityError(t, e.api.Post("/auth/universities", e.bearer(t, RoleAdmin), universityBody(id, "student.kul.pl")), http.StatusConflict, "domain_taken")
		var n int
		if err := e.pool.QueryRow(context.Background(), "SELECT count(*) FROM universities WHERE id = $1", id).Scan(&n); err != nil || n != 0 {
			t.Errorf("rows = %d, %v; want 0", n, err)
		}
	})

	t.Run("a new university cannot name faculty ids", func(t *testing.T) {
		body := universityBody(e.newID(t), newDomain())
		body["faculties"] = []map[string]any{{"id": uuid.New(), "name": "Wydział Pierwszy", "address": "ul. Testowa 1", "latitude": 51.2, "longitude": 22.5}}
		expectUniversityError(t, e.api.Post("/auth/universities", e.bearer(t, RoleAdmin), body), http.StatusBadRequest, "invalid_input")
	})

	t.Run("an OTP university without domains is invalid", func(t *testing.T) {
		body := universityBody(e.newID(t), newDomain())
		body["domains"] = []string{}
		expectUniversityError(t, e.api.Post("/auth/universities", e.bearer(t, RoleAdmin), body), http.StatusBadRequest, "invalid_input")
	})

	t.Run("two admins adding the same id at once create it once", func(t *testing.T) {
		admin := e.bearer(t, RoleAdmin)
		id := e.newID(t)
		var wg sync.WaitGroup
		codes := make([]int, 2)
		for i := range codes {
			wg.Go(func() {
				codes[i] = e.api.Post("/auth/universities", admin, universityBody(id, newDomain())).Code
			})
		}
		wg.Wait()
		slices.Sort(codes)
		if codes[0] != http.StatusCreated || codes[1] != http.StatusConflict {
			t.Errorf("codes = %v, want one 201 and one 409", codes)
		}
		var n int
		if err := e.pool.QueryRow(context.Background(), "SELECT count(*) FROM university_domains WHERE university_id = $1", id).Scan(&n); err != nil || n != 1 {
			t.Errorf("domains = %d, %v; want 1", n, err)
		}
	})
}

func TestReplaceUniversity(t *testing.T) {
	e := newUniversitiesEnv(t)

	create := func(t *testing.T) (id, domain string) {
		t.Helper()
		id, domain = e.newID(t), newDomain()
		decodeUniversity[University](t, e.api.Post("/auth/universities", e.bearer(t, RoleAdmin), universityBody(id, domain)), http.StatusCreated)
		return id, domain
	}

	t.Run("requires a session", func(t *testing.T) {
		id, _ := create(t)
		expectUniversityError(t, e.api.Put("/auth/universities/"+id, universityBody(id, newDomain())), http.StatusUnauthorized, "unauthorized")
	})

	t.Run("a student cannot change a university", func(t *testing.T) {
		id, _ := create(t)
		expectUniversityError(t, e.api.Put("/auth/universities/"+id, e.bearer(t), universityBody(id, newDomain())), http.StatusForbidden, "forbidden")
	})

	t.Run("replaces the name, domains and faculties", func(t *testing.T) {
		id, oldDomain := create(t)
		newOne := newDomain()
		radon := uuid.New()
		body := universityBody(id, newOne)
		delete(body, "id")
		body["name"] = "Nowa Nazwa Uczelni"
		body["domains"] = []string{oldDomain, newOne}
		body["radonInstitutionId"] = radon
		body["faculties"] = []map[string]any{{"name": "Wydział Jedyny", "address": "ul. Nowa 3", "latitude": 51.1, "longitude": 22.4}}

		got := decodeUniversity[University](t, e.api.Put("/auth/universities/"+id, e.bearer(t, RoleAdmin), body), http.StatusOK)
		if got.Name != "Nowa Nazwa Uczelni" || len(got.Domains) != 2 || len(got.Faculties) != 1 || got.Faculties[0].Name != "Wydział Jedyny" ||
			got.RadonInstitutionID == nil || *got.RadonInstitutionID != radon {
			t.Errorf("replaced = %+v", got)
		}
	})

	t.Run("a faculty sent with its id keeps the id and takes the new fields and order", func(t *testing.T) {
		id, domain := create(t)
		before := decodeUniversity[University](t, e.api.Get("/auth/universities/"+id), http.StatusOK)
		body := universityBody(id, domain)
		delete(body, "id")
		body["faculties"] = []map[string]any{
			{"name": "Wydział Nowy", "address": "ul. Nowa 1", "latitude": 51.0, "longitude": 22.0},
			{"id": before.Faculties[1].ID, "name": "Wydział Drugi Inny", "address": "ul. Inna 2", "latitude": 51.4, "longitude": 22.7},
		}

		got := decodeUniversity[University](t, e.api.Put("/auth/universities/"+id, e.bearer(t, RoleAdmin), body), http.StatusOK)
		if len(got.Faculties) != 2 || got.Faculties[0].Name != "Wydział Nowy" ||
			got.Faculties[1].ID != before.Faculties[1].ID || got.Faculties[1].Name != "Wydział Drugi Inny" {
			t.Errorf("faculties = %+v, kept id %s", got.Faculties, before.Faculties[1].ID)
		}
		if got.Faculties[0].ID == before.Faculties[0].ID {
			t.Errorf("the left out faculty %s was kept", before.Faculties[0].ID)
		}
	})

	t.Run("a faculty id of another university is invalid and changes nothing", func(t *testing.T) {
		id, domain := create(t)
		other, _ := create(t)
		foreign := decodeUniversity[University](t, e.api.Get("/auth/universities/"+other), http.StatusOK).Faculties[0].ID
		body := universityBody(id, domain)
		delete(body, "id")
		body["faculties"] = []map[string]any{{"id": foreign, "name": "Wydział Cudzy", "address": "ul. Obca 1", "latitude": 51.0, "longitude": 22.0}}

		expectUniversityError(t, e.api.Put("/auth/universities/"+id, e.bearer(t, RoleAdmin), body), http.StatusBadRequest, "invalid_input")
		if got := decodeUniversity[University](t, e.api.Get("/auth/universities/"+id), http.StatusOK); len(got.Faculties) != 2 {
			t.Errorf("faculties = %+v", got.Faculties)
		}
	})

	t.Run("a faculty that courses refer to cannot be left out", func(t *testing.T) {
		id, domain := create(t)
		faculty := decodeUniversity[University](t, e.api.Get("/auth/universities/"+id), http.StatusOK).Faculties[0].ID
		var course uuid.UUID
		err := e.pool.QueryRow(context.Background(), `
			INSERT INTO courses (name, description, difficulty, university_id, faculty_id)
			VALUES ('Analiza', 'Granice.', 3, $1, $2) RETURNING id`, id, faculty).Scan(&course)
		if err != nil {
			t.Fatalf("insert course: %v", err)
		}
		t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), "DELETE FROM courses WHERE id = $1", course) })
		body := universityBody(id, domain)
		delete(body, "id")
		body["faculties"] = []map[string]any{{"name": "Wydział Inny", "address": "ul. Inna 1", "latitude": 51.0, "longitude": 22.0}}

		expectUniversityError(t, e.api.Put("/auth/universities/"+id, e.bearer(t, RoleAdmin), body), http.StatusConflict, "faculty_in_use")
		if got := decodeUniversity[University](t, e.api.Get("/auth/universities/"+id), http.StatusOK); len(got.Faculties) != 2 {
			t.Errorf("faculties = %+v", got.Faculties)
		}
	})

	t.Run("the same faculty id twice is invalid", func(t *testing.T) {
		id, domain := create(t)
		kept := decodeUniversity[University](t, e.api.Get("/auth/universities/"+id), http.StatusOK).Faculties[0].ID
		body := universityBody(id, domain)
		delete(body, "id")
		body["faculties"] = []map[string]any{
			{"id": kept, "name": "Wydział Raz", "address": "ul. Testowa 1", "latitude": 51.0, "longitude": 22.0},
			{"id": kept, "name": "Wydział Dwa", "address": "ul. Testowa 2", "latitude": 51.0, "longitude": 22.0},
		}
		expectUniversityError(t, e.api.Put("/auth/universities/"+id, e.bearer(t, RoleAdmin), body), http.StatusBadRequest, "invalid_input")
	})

	t.Run("sending the current university again changes nothing", func(t *testing.T) {
		id, domain := create(t)
		body := universityBody(id, domain)
		delete(body, "id")
		got := decodeUniversity[University](t, e.api.Put("/auth/universities/"+id, e.bearer(t, RoleAdmin), body), http.StatusOK)
		if strings.Join(got.Domains, ",") != domain || len(got.Faculties) != 2 {
			t.Errorf("replaced = %+v", got)
		}
	})

	t.Run("a domain of another university is a conflict and changes nothing", func(t *testing.T) {
		id, domain := create(t)
		body := universityBody(id, "student.kul.pl")
		delete(body, "id")
		expectUniversityError(t, e.api.Put("/auth/universities/"+id, e.bearer(t, RoleAdmin), body), http.StatusConflict, "domain_taken")

		got := decodeUniversity[University](t, e.api.Get("/auth/universities/"+id), http.StatusOK)
		if strings.Join(got.Domains, ",") != domain {
			t.Errorf("domains = %v, want %s", got.Domains, domain)
		}
	})

	t.Run("an unknown university is not found", func(t *testing.T) {
		body := universityBody("nowhere", newDomain())
		delete(body, "id")
		expectUniversityError(t, e.api.Put("/auth/universities/nowhere", e.bearer(t, RoleAdmin), body), http.StatusNotFound, "not_found")
	})
}
