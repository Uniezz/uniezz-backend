package study

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
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/google/uuid"
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

type testEnv struct {
	pool   *pgxpool.Pool
	api    humatest.TestAPI
	module Module
	media  *fakeMedia
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	pool := setupTestDB(t)
	usos := map[string]config.UsosCredentials{}
	for _, u := range auth.GetUniversities() {
		if u.AuthType == auth.AuthTypeUSOS {
			usos[string(u.ID)] = config.UsosCredentials{BaseURL: "https://usos.test", ConsumerKey: "key", ConsumerSecret: "secret"}
		}
	}
	authModule, err := auth.New(pool, &config.Config{UsosConfigs: usos}, nil, nil)
	if err != nil {
		t.Fatalf("auth.New failed: %v", err)
	}
	_, api := humatest.New(t, httpapi.NewConfig(&config.Config{}))
	api.UseMiddleware(authModule.Guard(api))
	fake := &fakeMedia{pool: pool}
	module, err := New(pool, fake, jobs.NewRunner(pool, 1, time.Hour))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	module.Register(api)
	return &testEnv{pool: pool, api: api, module: module, media: fake}
}

type student struct {
	id     uuid.UUID
	bearer string
}

func (e *testEnv) student(t *testing.T) student {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	err := e.pool.QueryRow(ctx,
		"INSERT INTO users (university_id, email, first_name, last_name) VALUES ('umcs', $1, 'Jan', 'Kowalski') RETURNING id",
		"study-"+uuid.NewString()+"@example.test").Scan(&id)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		for _, query := range []string{
			"DELETE FROM course_files WHERE author_id = $1",
			"DELETE FROM course_comments WHERE author_id = $1",
			"DELETE FROM media WHERE owner_user_id = $1",
			"DELETE FROM users WHERE id = $1",
		} {
			_, _ = e.pool.Exec(context.Background(), query, id)
		}
	})

	token := auth.GenerateSessionToken()
	hash := token.Hash()
	if _, err := auth.NewSessionRepository(database.New(e.pool)).CreateSession(ctx, id, hash[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return student{id: id, bearer: "Authorization: Bearer " + token.String()}
}

func (e *testEnv) block(t *testing.T, blocker, blocked uuid.UUID) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), "INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)", blocker, blocked); err != nil {
		t.Fatalf("block: %v", err)
	}
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM blocks WHERE blocker_id = $1 AND blocked_id = $2", blocker, blocked)
	})
}

func (e *testEnv) course(t *testing.T, author student, overrides map[string]any) Course {
	t.Helper()
	body := map[string]any{
		"name":         "Calculus " + uuid.NewString()[:8],
		"description":  "Limits and derivatives.",
		"difficulty":   3,
		"universityId": "umcs",
		"faculty":      "Mathematics",
		"subject":      "Analysis " + uuid.NewString()[:8],
		"semester":     2,
	}
	for k, v := range overrides {
		body[k] = v
	}
	course := decode[Course](t, e.api.Post("/study/courses", author.bearer, body), 201)
	e.cleanupCourse(t, course.ID)
	return course
}

func (e *testEnv) cleanupCourse(t *testing.T, id uuid.UUID) {
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = e.pool.Exec(ctx, "DELETE FROM course_files WHERE course_id = $1", id)
		_, _ = e.pool.Exec(ctx, "DELETE FROM course_comments WHERE course_id = $1", id)
		_, _ = e.pool.Exec(ctx, "DELETE FROM courses WHERE id = $1", id)
	})
}

func (e *testEnv) upload(t *testing.T, owner uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := e.pool.Exec(context.Background(), `
		INSERT INTO media (id, owner_user_id, kind, object_key, content_type, size_bytes)
		VALUES ($1, $2, 'exam', $3, 'application/pdf', 10)`, id, owner, "exams/"+id.String()+".pdf")
	if err != nil {
		t.Fatalf("insert media: %v", err)
	}
	return id
}

func (e *testEnv) file(t *testing.T, author student, courseID uuid.UUID, title string) CourseFile {
	t.Helper()
	body := map[string]any{"mediaId": e.upload(t, author.id), "title": title, "type": "exam"}
	return decode[CourseFile](t, e.api.Post("/study/courses/"+courseID.String()+"/files", author.bearer, body), 201)
}

type fakeMedia struct {
	pool    *pgxpool.Pool
	deleted []uuid.UUID
}

func (f *fakeMedia) CheckUpload(ctx context.Context, mediaID, ownerUserID uuid.UUID, kind media.Kind) error {
	var found bool
	err := f.pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM media WHERE id = $1 AND owner_user_id = $2 AND kind = $3 AND status = 'pending')",
		mediaID, ownerUserID, kind).Scan(&found)
	if err != nil {
		return err
	}
	if !found {
		return media.ErrNotFound
	}
	return nil
}

func (f *fakeMedia) MarkReady(ctx context.Context, mediaID, ownerUserID uuid.UUID, kind media.Kind) error {
	tag, err := database.New(f.pool).Conn(ctx).Exec(ctx,
		"UPDATE media SET status = 'ready' WHERE id = $1 AND owner_user_id = $2 AND kind = $3 AND status = 'pending'",
		mediaID, ownerUserID, kind)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return media.ErrNotFound
	}
	return nil
}

func (f *fakeMedia) MarkDeleting(ctx context.Context, mediaID uuid.UUID) error {
	tag, err := database.New(f.pool).Conn(ctx).Exec(ctx, "UPDATE media SET status = 'deleting' WHERE id = $1", mediaID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return media.ErrNotFound
	}
	return nil
}

func (f *fakeMedia) Delete(ctx context.Context, mediaID uuid.UUID) error {
	if _, err := f.pool.Exec(ctx, "DELETE FROM media WHERE id = $1 AND status = 'deleting'", mediaID); err != nil {
		return err
	}
	f.deleted = append(f.deleted, mediaID)
	return nil
}

func (f *fakeMedia) URLOf(_ context.Context, ref media.Ref) (string, error) {
	return "https://media.test/" + ref.ObjectKey, nil
}

type itemPage[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"nextCursor"`
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
	body := decode[map[string]any](t, rec, wantStatus)
	if body["error"] != wantCode {
		t.Errorf(`expected {"error":%q}, got %s`, wantCode, rec.Body.String())
	}
}
