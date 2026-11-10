package connections

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/Uniezz/uniezz-backend/internal/profile"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMatch(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip()
	}
	ctx := context.Background()
	pool, err := database.NewPool(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	usos := map[string]config.UsosCredentials{}
	for _, u := range auth.GetUniversities() {
		if u.AuthType == auth.AuthTypeUSOS {
			usos[string(u.ID)] = config.UsosCredentials{BaseURL: "https://usos.test", ConsumerKey: "key", ConsumerSecret: "secret"}
		}
	}
	authModule, err := auth.New(pool, &config.Config{UsosConfigs: usos}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := profile.New(pool, media.Module{}, jobs.NewRunner(pool, 1, time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	module := New(pool, profiles)
	_, api := humatest.New(t, httpapi.NewConfig(&config.Config{}))
	api.UseMiddleware(authModule.Guard(api))
	module.Register(api)

	a := user(t, pool)
	b := user(t, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = ANY($1)", []uuid.UUID{a.id, b.id})
	})

	prefs := map[string]any{"birthDate": "2000-01-01", "openTo": []string{"friends"}, "intent": "Climbing"}
	if rec := api.Patch("/connections/me", a.bearer, prefs); rec.Code != 204 && rec.Code != 200 {
		t.Fatalf("prefs a: %d %s", rec.Code, rec.Body.String())
	}
	if rec := api.Patch("/connections/me", b.bearer, prefs); rec.Code != 204 && rec.Code != 200 {
		t.Fatalf("prefs b: %d %s", rec.Code, rec.Body.String())
	}

	card := discoverOne(t, api, a.bearer)
	if card.CandidateID == b.id || card.Age == nil || *card.Age < 16 || card.UniversityID != "umcs" {
		t.Fatalf("card leaks or is wrong: %+v user %s", card, b.id)
	}
	other := discoverOne(t, api, b.bearer)
	if rec := api.Put("/connections/candidates/"+other.CandidateID.String(), b.bearer, map[string]any{"mode": "friends", "action": "like"}); rec.Code != 200 {
		t.Fatalf("like b: %d %s", rec.Code, rec.Body.String())
	}
	rec := api.Put("/connections/candidates/"+card.CandidateID.String(), a.bearer, map[string]any{"mode": "friends", "action": "like"})
	var matched actionResult
	decode(t, rec, 200, &matched)
	if !matched.Matched || matched.Connection == nil || matched.Connection.Profile == nil || matched.Connection.Profile.ID != b.id {
		t.Fatalf("match = %+v", matched)
	}
	if rec := api.Put("/connections/candidates/"+card.CandidateID.String(), b.bearer, map[string]any{"mode": "friends", "action": "like"}); rec.Code != 404 {
		t.Fatalf("other viewer candidate: %d %s", rec.Code, rec.Body.String())
	}
}

type testUser struct {
	id     uuid.UUID
	bearer string
}

func user(t *testing.T, pool *pgxpool.Pool) testUser {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO users (university_id, email, first_name, last_name, bio, interests)
		VALUES ('umcs', $1, 'Jan', 'Kowalski', 'Bio', ARRAY['climbing']) RETURNING id`,
		"conn-"+uuid.NewString()+"@example.test").Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	token := auth.GenerateSessionToken()
	hash := token.Hash()
	if _, err := auth.NewSessionRepository(database.New(pool)).CreateSession(ctx, id, hash[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	return testUser{id: id, bearer: "Authorization: Bearer " + token.String()}
}

func discoverOne(t *testing.T, api humatest.TestAPI, bearer string) Card {
	t.Helper()
	rec := api.Get("/connections/discover?mode=friends&limit=1", bearer)
	var body struct {
		Items []Card `json:"items"`
	}
	decode(t, rec, 200, &body)
	if len(body.Items) != 1 {
		t.Fatalf("discover = %+v", body.Items)
	}
	return body.Items[0]
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, status int, dest any) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("expected %d, got %d: %s", status, rec.Code, rec.Body.String())
	}
	if dest == nil {
		return
	}
	if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
		t.Fatal(err)
	}
}
