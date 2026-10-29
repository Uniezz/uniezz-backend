package profile

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping database integration test in short mode")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("Skipping database integration test because DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to create database pool: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

// fakeMedia keeps media rows in the real table, so the avatar foreign key holds,
// and skips object storage.
type fakeMedia struct {
	pool      *pgxpool.Pool
	deleted   []uuid.UUID
	imported  []string
	importErr error
	onImport  func()
}

func (f *fakeMedia) Claim(ctx context.Context, mediaID, ownerUserID uuid.UUID, kind media.Kind) error {
	result, err := f.pool.Exec(ctx,
		"UPDATE media SET status = 'ready' WHERE id = $1 AND owner_user_id = $2 AND kind = $3 AND status = 'pending'",
		mediaID, ownerUserID, kind)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return media.ErrNotFound
	}
	return nil
}

func (f *fakeMedia) URL(_ context.Context, mediaID uuid.UUID) (string, error) {
	return "https://media.test/" + mediaID.String(), nil
}

func (f *fakeMedia) MarkDeleting(ctx context.Context, tx pgx.Tx, mediaID uuid.UUID) error {
	result, err := tx.Exec(ctx, "UPDATE media SET status = 'deleting' WHERE id = $1 AND status = 'ready'", mediaID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return media.ErrNotFound
	}
	return nil
}

func (f *fakeMedia) Delete(ctx context.Context, mediaID uuid.UUID) error {
	f.deleted = append(f.deleted, mediaID)
	_, err := f.pool.Exec(ctx, "DELETE FROM media WHERE id = $1 AND status = 'deleting'", mediaID)
	return err
}

func (f *fakeMedia) Import(ctx context.Context, ownerUserID uuid.UUID, kind media.Kind, sourceURL string) (uuid.UUID, error) {
	f.imported = append(f.imported, sourceURL)
	if f.importErr != nil {
		return uuid.Nil, f.importErr
	}
	id := insertMedia(ctx, f.pool, ownerUserID, "ready")
	if f.onImport != nil {
		f.onImport()
	}
	return id, nil
}

func insertMedia(ctx context.Context, pool *pgxpool.Pool, ownerID uuid.UUID, status string) uuid.UUID {
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO media (owner_user_id, kind, object_key, content_type, size_bytes, status)
		VALUES ($1, 'avatar', 'avatars/' || gen_random_uuid() || '.jpg', 'image/jpeg', 10, $2)
		RETURNING id`, ownerID, status).Scan(&id)
	if err != nil {
		panic(err)
	}
	return id
}

func mediaStatus(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) string {
	t.Helper()

	var s string
	err := pool.QueryRow(context.Background(), "SELECT status FROM media WHERE id = $1", id).Scan(&s)
	if errors.Is(err, pgx.ErrNoRows) {
		return "gone"
	}
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type testEnv struct {
	pool   *pgxpool.Pool
	api    humatest.TestAPI
	media  *fakeMedia
	module Module
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	pool := setupTestDB(t)
	fm := &fakeMedia{pool: pool}
	runner := jobs.NewRunner(pool, 1)
	m, err := New(pool, fm, runner)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	authModule, err := auth.New(pool, &config.Config{UsosConfigs: map[string]config.UsosCredentials{
		"umcs":  {BaseURL: "https://usos.test", ConsumerKey: "key", ConsumerSecret: "secret"},
		"umlub": {BaseURL: "https://usos.test", ConsumerKey: "key", ConsumerSecret: "secret"},
	}}, nil, nil)
	if err != nil {
		t.Fatalf("auth.New failed: %v", err)
	}

	_, api := humatest.New(t, httpapi.NewConfig(&config.Config{}))
	api.UseMiddleware(authModule.Guard(api))
	m.Register(api)

	return &testEnv{pool: pool, api: api, media: fm, module: m}
}

type testUser struct {
	id     uuid.UUID
	bearer string
}

func (e *testEnv) newUser(t *testing.T, university, email string) testUser {
	t.Helper()
	ctx := context.Background()

	var id uuid.UUID
	err := e.pool.QueryRow(ctx,
		"INSERT INTO users (university_id, email, first_name, last_name) VALUES ($1, $2, 'Jan', 'Kowalski') RETURNING id",
		university, email).Scan(&id)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id)
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM jobs WHERE payload->>'userId' = $1", id.String())
	})

	token := auth.GenerateSessionToken()
	hash := token.Hash()
	if _, err := auth.NewSessionRepository(e.pool).CreateSession(ctx, id, hash[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return testUser{id: id, bearer: "Authorization: Bearer " + token.String()}
}

func (e *testEnv) otpUser(t *testing.T) testUser {
	return e.newUser(t, "kul", "profile-"+uuid.NewString()+"@student.kul.pl")
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) T {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("expected %d, got %d: %s", wantStatus, rec.Code, rec.Body.String())
	}
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, rec.Body.String())
	}
	return v
}

func expectError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()

	body := decode[map[string]string](t, rec, wantStatus)
	if body["error"] != wantCode {
		t.Errorf(`expected {"error":%q}, got %s`, wantCode, rec.Body.String())
	}
}

func TestGetMe(t *testing.T) {
	e := newTestEnv(t)

	t.Run("has the same keys for every account, with an empty programme list", func(t *testing.T) {
		u := e.otpUser(t)

		body := decode[map[string]any](t, e.api.Get("/profile/me", u.bearer), http.StatusOK)

		var keys []string
		for k := range body {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		want := []string{"avatarUrl", "createdAt", "email", "firstName", "id", "lastName", "programmes", "universityId"}
		if !slices.Equal(keys, want) {
			t.Errorf("expected keys %v, got %v", want, keys)
		}
		if body["id"] != u.id.String() || body["universityId"] != "kul" || body["firstName"] != "Jan" {
			t.Errorf("unexpected profile %v", body)
		}
		if programmes, ok := body["programmes"].([]any); !ok || len(programmes) != 0 {
			t.Errorf("expected programmes [], got %v", body["programmes"])
		}
		if body["avatarUrl"] != nil {
			t.Errorf("expected avatarUrl null, got %v", body["avatarUrl"])
		}
	})

	t.Run("lists every programme, primary first, and the avatar URL", func(t *testing.T) {
		u := e.newUser(t, "umcs", "profile-"+uuid.NewString()+"@umcs.pl")
		ctx := context.Background()

		err := pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
			_, err := e.module.SeedAccount(ctx, tx, u.id, auth.AccountSeed{Programmes: []auth.ProgrammeSeed{
				{Faculty: "Wydział Prawa", Programme: "Prawo", Level: "long_cycle"},
				{Faculty: "Wydział Matematyki, Fizyki i Informatyki", Programme: "Informatyka", ModeOfStudies: "full_time", IsPrimary: true},
			}})
			return err
		})
		if err != nil {
			t.Fatalf("SeedAccount failed: %v", err)
		}
		avatarID := insertMedia(ctx, e.pool, u.id, "ready")
		if _, err := e.pool.Exec(ctx, "UPDATE users SET avatar_media_id = $2 WHERE id = $1", u.id, avatarID); err != nil {
			t.Fatal(err)
		}

		body := decode[ProfileDTO](t, e.api.Get("/profile/me", u.bearer), http.StatusOK)

		if len(body.Programmes) != 2 || body.Programmes[0].Programme != "Informatyka" || !body.Programmes[0].IsPrimary {
			t.Fatalf("expected two programmes with Informatyka primary first, got %+v", body.Programmes)
		}
		second := body.Programmes[1]
		if second.Faculty != "Wydział Prawa" || second.Level == nil || *second.Level != "long_cycle" || second.Status != "active" || second.YearOfStudy != nil {
			t.Errorf("unexpected second programme %+v", second)
		}
		if body.AvatarURL == nil || *body.AvatarURL != "https://media.test/"+avatarID.String() {
			t.Errorf("expected the avatar URL, got %v", body.AvatarURL)
		}
	})

	t.Run("needs a session", func(t *testing.T) {
		expectError(t, e.api.Get("/profile/me"), http.StatusUnauthorized, "unauthorized")
	})
}

func TestUpdateMe(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t.Run("changes only the names that are sent, trimmed", func(t *testing.T) {
		u := e.otpUser(t)

		body := decode[ProfileDTO](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"firstName": "  Anna "}), http.StatusOK)

		if *body.FirstName != "Anna" || *body.LastName != "Kowalski" {
			t.Errorf("expected Anna Kowalski, got %q %q", *body.FirstName, *body.LastName)
		}
	})

	t.Run("a blank name is a 400", func(t *testing.T) {
		u := e.otpUser(t)
		expectError(t, e.api.Patch("/profile/me", u.bearer, map[string]any{"lastName": "   "}), http.StatusBadRequest, "invalid_input")
	})

	t.Run("sets an uploaded avatar, then replaces it and deletes the old one", func(t *testing.T) {
		u := e.otpUser(t)
		first := insertMedia(ctx, e.pool, u.id, "pending")
		second := insertMedia(ctx, e.pool, u.id, "pending")

		body := decode[ProfileDTO](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"avatarMediaId": first.String()}), http.StatusOK)
		if body.AvatarURL == nil || *body.AvatarURL != "https://media.test/"+first.String() {
			t.Fatalf("expected the first avatar, got %v", body.AvatarURL)
		}

		body = decode[ProfileDTO](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"avatarMediaId": second.String()}), http.StatusOK)
		if body.AvatarURL == nil || *body.AvatarURL != "https://media.test/"+second.String() {
			t.Fatalf("expected the second avatar, got %v", body.AvatarURL)
		}
		if !slices.Contains(e.media.deleted, first) || mediaStatus(t, e.pool, first) != "gone" {
			t.Errorf("expected the first avatar deleted, deleted=%v status=%s", e.media.deleted, mediaStatus(t, e.pool, first))
		}
	})

	t.Run("sending the current avatar again changes nothing", func(t *testing.T) {
		u := e.otpUser(t)
		avatar := insertMedia(ctx, e.pool, u.id, "pending")
		decode[ProfileDTO](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"avatarMediaId": avatar.String()}), http.StatusOK)

		decode[ProfileDTO](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"avatarMediaId": avatar.String()}), http.StatusOK)

		if mediaStatus(t, e.pool, avatar) != "ready" {
			t.Errorf("expected the avatar kept, got %s", mediaStatus(t, e.pool, avatar))
		}
	})

	t.Run("someone else's or unknown media is a 400", func(t *testing.T) {
		u := e.otpUser(t)
		other := e.otpUser(t)
		foreign := insertMedia(ctx, e.pool, other.id, "pending")

		for _, id := range []uuid.UUID{foreign, uuid.New()} {
			rec := e.api.Patch("/profile/me", u.bearer, map[string]any{"avatarMediaId": id.String()})
			expectError(t, rec, http.StatusBadRequest, "invalid_avatar")
		}
		if mediaStatus(t, e.pool, foreign) != "pending" {
			t.Error("someone else's media must stay untouched")
		}
	})
}

func TestDeleteAvatar(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	u := e.otpUser(t)
	avatar := insertMedia(ctx, e.pool, u.id, "pending")
	decode[ProfileDTO](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"avatarMediaId": avatar.String()}), http.StatusOK)

	if rec := e.api.Delete("/profile/avatar", u.bearer); rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	body := decode[ProfileDTO](t, e.api.Get("/profile/me", u.bearer), http.StatusOK)
	if body.AvatarURL != nil {
		t.Errorf("expected no avatar, got %v", *body.AvatarURL)
	}
	if mediaStatus(t, e.pool, avatar) != "gone" {
		t.Errorf("expected the avatar media deleted, got %s", mediaStatus(t, e.pool, avatar))
	}

	if rec := e.api.Delete("/profile/avatar", u.bearer); rec.Code != http.StatusNoContent {
		t.Errorf("deleting a missing avatar should still be 204, got %d", rec.Code)
	}
}

func TestProgrammes(t *testing.T) {
	e := newTestEnv(t)

	create := func(t *testing.T, u testUser, body map[string]any) ProgrammeDTO {
		t.Helper()
		return decode[ProgrammeDTO](t, e.api.Post("/profile/programmes", u.bearer, body), http.StatusCreated)
	}
	list := func(t *testing.T, u testUser) []ProgrammeDTO {
		t.Helper()
		return decode[struct{ Programmes []ProgrammeDTO }](t, e.api.Get("/profile/programmes", u.bearer), http.StatusOK).Programmes
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

		rec := e.api.Put("/profile/programmes/"+p.ID, u.bearer, map[string]any{"faculty": "A", "programme": "C", "status": "graduated"})
		got := decode[ProgrammeDTO](t, rec, http.StatusOK)

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

		expectError(t, e.api.Put("/profile/programmes/"+p.ID, intruder.bearer, map[string]any{"faculty": "X", "programme": "Y"}),
			http.StatusNotFound, "not_found")
		expectError(t, e.api.Delete("/profile/programmes/"+p.ID, intruder.bearer), http.StatusNotFound, "not_found")

		if got := list(t, owner); len(got) != 1 || got[0].Programme != "B" {
			t.Errorf("owner's programme must be untouched, got %+v", got)
		}
	})

	t.Run("deletes", func(t *testing.T) {
		u := e.otpUser(t)
		p := create(t, u, map[string]any{"faculty": "A", "programme": "B"})

		if rec := e.api.Delete("/profile/programmes/"+p.ID, u.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := list(t, u); len(got) != 0 {
			t.Errorf("expected no programmes, got %+v", got)
		}
	})
}

func TestSeedAccount(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	u := e.otpUser(t)

	var afterCommit func()
	err := pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
		var err error
		afterCommit, err = e.module.SeedAccount(ctx, tx, u.id, auth.AccountSeed{PhotoURL: "https://usos.test/photo.jpg"})
		return err
	})
	if err != nil {
		t.Fatalf("SeedAccount failed: %v", err)
	}
	if afterCommit == nil {
		t.Error("expected a wakeup to run after commit")
	}

	var payload ImportAvatarArgs
	var raw []byte
	err = e.pool.QueryRow(ctx, "SELECT payload FROM jobs WHERE kind = $1 AND payload->>'userId' = $2", ImportAvatar.Kind(), u.id.String()).Scan(&raw)
	if err != nil {
		t.Fatalf("expected an import job: %v", err)
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.SourceURL != "https://usos.test/photo.jpg" {
		t.Errorf("unexpected payload %s", raw)
	}
}

func TestImportAvatar(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	avatarOf := func(t *testing.T, id uuid.UUID) *uuid.UUID {
		t.Helper()
		var avatar *uuid.UUID
		if err := e.pool.QueryRow(ctx, "SELECT avatar_media_id FROM users WHERE id = $1", id).Scan(&avatar); err != nil {
			t.Fatal(err)
		}
		return avatar
	}

	t.Run("sets the imported photo as the avatar", func(t *testing.T) {
		u := e.otpUser(t)

		if err := e.module.importAvatar(ctx, ImportAvatarArgs{UserID: u.id, SourceURL: "https://usos.test/p.jpg"}); err != nil {
			t.Fatalf("importAvatar failed: %v", err)
		}
		if avatarOf(t, u.id) == nil {
			t.Error("expected an avatar")
		}
	})

	t.Run("a repeated run keeps the avatar and imports nothing", func(t *testing.T) {
		u := e.otpUser(t)
		args := ImportAvatarArgs{UserID: u.id, SourceURL: "https://usos.test/p.jpg"}
		if err := e.module.importAvatar(ctx, args); err != nil {
			t.Fatal(err)
		}
		first := avatarOf(t, u.id)
		imports := len(e.media.imported)

		if err := e.module.importAvatar(ctx, args); err != nil {
			t.Fatal(err)
		}
		if *avatarOf(t, u.id) != *first || len(e.media.imported) != imports {
			t.Error("a second run must not replace the avatar or download again")
		}
	})

	t.Run("an avatar chosen during the import wins and the import is discarded", func(t *testing.T) {
		u := e.otpUser(t)
		chosen := insertMedia(ctx, e.pool, u.id, "ready")
		e.media.onImport = func() {
			_, _ = e.pool.Exec(ctx, "UPDATE users SET avatar_media_id = $2 WHERE id = $1", u.id, chosen)
		}
		defer func() { e.media.onImport = nil }()
		deletedBefore := len(e.media.deleted)

		if err := e.module.importAvatar(ctx, ImportAvatarArgs{UserID: u.id, SourceURL: "https://usos.test/p.jpg"}); err != nil {
			t.Fatal(err)
		}
		if got := avatarOf(t, u.id); got == nil || *got != chosen {
			t.Errorf("expected the chosen avatar kept, got %v", got)
		}
		if len(e.media.deleted) != deletedBefore+1 {
			t.Error("expected the imported media deleted")
		}
	})

	t.Run("an image USOS cannot give us is a permanent failure", func(t *testing.T) {
		u := e.otpUser(t)
		e.media.importErr = media.ErrInvalidUpload
		defer func() { e.media.importErr = nil }()

		err := e.module.importAvatar(ctx, ImportAvatarArgs{UserID: u.id, SourceURL: "https://usos.test/p.html"})
		if err == nil || !errors.Is(err, media.ErrInvalidUpload) {
			t.Fatalf("expected the invalid upload error, got %v", err)
		}
	})
}
