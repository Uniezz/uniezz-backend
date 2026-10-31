package media

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/database"
)

func newImageServer(t *testing.T, contentType string, body []byte) *httptest.Server {
	t.Helper()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestImport(t *testing.T) {
	pool := setupTestDB(t)
	repo := newRepository(database.New(pool))
	ownerID := createTestUser(t, pool)
	ctx := context.Background()

	t.Run("stores the image and returns ready media", func(t *testing.T) {
		image := []byte("\xff\xd8\xff fake jpeg")
		srv := newImageServer(t, "image/jpeg; charset=binary", image)
		objects := &fakeObjects{}
		m := Module{repo: repo, storage: objects, httpClient: srv.Client()}

		id, err := m.Import(ctx, ownerID, KindAvatar, srv.URL+"/photo.jpg")
		if err != nil {
			t.Fatalf("Import failed: %v", err)
		}

		item, err := repo.getReady(ctx, id)
		if err != nil {
			t.Fatalf("expected ready media: %v", err)
		}
		if item.OwnerUserID != ownerID || item.ContentType != "image/jpeg" || item.SizeBytes != int64(len(image)) {
			t.Errorf("unexpected media %+v", item)
		}
		if !strings.HasPrefix(item.ObjectKey, "avatars/") || string(objects.put[item.ObjectKey]) != string(image) {
			t.Errorf("expected the image stored under %q, got %v", item.ObjectKey, objects.put)
		}
	})

	for name, tc := range map[string]struct {
		contentType string
		body        []byte
	}{
		"a non-image response":    {"text/html", []byte("<html>login</html>")},
		"an image over the limit": {"image/png", make([]byte, 5<<20+1)},
	} {
		t.Run(name+" is rejected as invalid", func(t *testing.T) {
			srv := newImageServer(t, tc.contentType, tc.body)
			objects := &fakeObjects{}
			m := Module{repo: repo, storage: objects, httpClient: srv.Client()}

			if _, err := m.Import(ctx, ownerID, KindAvatar, srv.URL); !errors.Is(err, ErrInvalidUpload) {
				t.Fatalf("expected ErrInvalidUpload, got %v", err)
			}
			if len(objects.put) != 0 {
				t.Error("nothing must be stored for an invalid image")
			}
		})
	}

	t.Run("plain http sources are rejected", func(t *testing.T) {
		m := Module{repo: repo, storage: &fakeObjects{}, httpClient: http.DefaultClient}

		if _, err := m.Import(ctx, ownerID, KindAvatar, "http://usos.test/photo.jpg"); !errors.Is(err, ErrInvalidUpload) {
			t.Fatalf("expected ErrInvalidUpload, got %v", err)
		}
	})

	t.Run("a server error is temporary, not invalid", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		t.Cleanup(srv.Close)
		m := Module{repo: repo, storage: &fakeObjects{}, httpClient: srv.Client()}

		_, err := m.Import(ctx, ownerID, KindAvatar, srv.URL)
		if err == nil || errors.Is(err, ErrInvalidUpload) {
			t.Fatalf("expected a retryable error, got %v", err)
		}
	})
}
