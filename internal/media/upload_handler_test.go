package media

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newGuardedAPI(t *testing.T, pool *pgxpool.Pool) humatest.TestAPI {
	t.Helper()

	authModule, err := auth.New(pool, &config.Config{UsosConfigs: map[string]config.UsosCredentials{
		"umcs":  {BaseURL: "https://usos.test", ConsumerKey: "key", ConsumerSecret: "secret"},
		"umlub": {BaseURL: "https://usos.test", ConsumerKey: "key", ConsumerSecret: "secret"},
	}}, nil, nil)
	if err != nil {
		t.Fatalf("auth.New failed: %v", err)
	}

	_, api := humatest.New(t, httpapi.NewConfig(&config.Config{}))
	api.UseMiddleware(authModule.Guard(api))
	return api
}

func bearerFor(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) string {
	t.Helper()

	token := auth.GenerateSessionToken()
	hash := token.Hash()
	if _, err := auth.NewSessionRepository(pool).CreateSession(context.Background(), userID, hash[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	return "Authorization: Bearer " + token.String()
}

func TestUploadHandler(t *testing.T) {
	pool := setupTestDB(t)
	ownerID := createTestUser(t, pool)
	api := newGuardedAPI(t, pool)
	m := Module{storage: newTestStorage(t), repo: newRepository(pool)}
	m.Register(api)
	bearer := bearerFor(t, pool, ownerID)

	t.Run("creates a pending upload owned by the caller", func(t *testing.T) {
		rec := api.Post("/media/uploads", bearer, map[string]any{"kind": "avatar", "contentType": "image/png", "size": 2048})
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}

		var body struct {
			MediaID   string    `json:"mediaId"`
			UploadURL string    `json:"uploadUrl"`
			ExpiresAt time.Time `json:"expiresAt"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if !strings.HasPrefix(body.UploadURL, "http://localhost:9000/uniezz-media/avatars/") || body.ExpiresAt.IsZero() {
			t.Errorf("unexpected upload %+v", body)
		}

		item, err := newRepository(pool).getOwnedPending(context.Background(), uuid.MustParse(body.MediaID), ownerID, KindAvatar)
		if err != nil {
			t.Fatalf("expected a pending media row owned by the caller: %v", err)
		}
		if item.ContentType != "image/png" || item.SizeBytes != 2048 {
			t.Errorf("stored %q / %d, want image/png / 2048", item.ContentType, item.SizeBytes)
		}
	})

	t.Run("an avatar over the size limit is a 400", func(t *testing.T) {
		rec := api.Post("/media/uploads", bearer, map[string]any{"kind": "avatar", "contentType": "image/png", "size": 5<<20 + 1})
		expectError(t, rec.Code, rec.Body.Bytes(), http.StatusBadRequest, "invalid_upload")
	})

	t.Run("an unsupported content type is a 400", func(t *testing.T) {
		rec := api.Post("/media/uploads", bearer, map[string]any{"kind": "avatar", "contentType": "image/svg+xml", "size": 10})
		expectError(t, rec.Code, rec.Body.Bytes(), http.StatusBadRequest, "invalid_input")
	})

	t.Run("no session is a 401", func(t *testing.T) {
		rec := api.Post("/media/uploads", map[string]any{"kind": "avatar", "contentType": "image/png", "size": 10})
		expectError(t, rec.Code, rec.Body.Bytes(), http.StatusUnauthorized, "unauthorized")
	})
}

func expectError(t *testing.T, gotStatus int, body []byte, wantStatus int, wantCode string) {
	t.Helper()

	if gotStatus != wantStatus {
		t.Fatalf("expected %d, got %d: %s", wantStatus, gotStatus, body)
	}
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err != nil || e.Error != wantCode {
		t.Errorf(`expected {"error":%q}, got %s`, wantCode, body)
	}
}
