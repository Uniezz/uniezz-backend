package media

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type recordingStorage struct {
	*objectStorage
	deleted []string
}

func (s *recordingStorage) deleteObject(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}

func readyOf(t *testing.T, repo *repository, ownerID uuid.UUID, kind Kind, contentType string) *mediaRow {
	t.Helper()
	ctx := context.Background()
	item, err := repo.createPending(ctx, ownerID, kind, objectKey(kind, contentType), contentType, 10)
	if err != nil {
		t.Fatalf("create pending media: %v", err)
	}
	if err := repo.markReady(ctx, item.ID, ownerID, kind); err != nil {
		t.Fatalf("mark media ready: %v", err)
	}
	return item
}

func moderatorBearer(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	moderatorID := createTestUser(t, pool)
	authModule, err := auth.New(pool, &config.Config{}, nil, nil, nil)
	if err != nil {
		t.Fatalf("auth.New failed: %v", err)
	}
	if err := authModule.GrantRole(context.Background(), moderatorID, auth.RoleModerator); err != nil {
		t.Fatalf("grant moderator: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM user_roles WHERE user_id = $1", moderatorID)
	})
	return bearerFor(t, pool, moderatorID)
}

func TestImageReview(t *testing.T) {
	pool := setupTestDB(t)
	repo := newRepository(database.New(pool))
	storage := &recordingStorage{objectStorage: newTestStorage(t)}
	api := newGuardedAPI(t, pool)
	m := Module{storage: storage, repo: repo}
	m.Register(api)
	moderator := moderatorBearer(t, pool)

	listed := func(t *testing.T) []uuid.UUID {
		t.Helper()
		var ids []uuid.UUID
		cursor := ""
		for {
			path := "/media/images?limit=50"
			if cursor != "" {
				path += "&cursor=" + cursor
			}
			rec := api.Get(path, moderator)
			if rec.Code != http.StatusOK {
				t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
			}
			var page struct {
				Items      []Image
				NextCursor *string
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			for _, item := range page.Items {
				ids = append(ids, item.ID)
			}
			if page.NextCursor == nil {
				return ids
			}
			cursor = *page.NextCursor
		}
	}

	t.Run("students cannot open the queue and anonymous callers are unauthorized", func(t *testing.T) {
		student := bearerFor(t, pool, createTestUser(t, pool))
		if rec := api.Get("/media/images", student); rec.Code != http.StatusForbidden {
			t.Errorf("student list = %d, want 403", rec.Code)
		}
		id := uuid.NewString()
		for _, rec := range []int{
			api.Get("/media/images").Code,
			api.Put("/media/images/"+id+"/review", map[string]any{}).Code,
			api.Delete("/media/images/" + id).Code,
		} {
			if rec != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec)
			}
		}
	})

	t.Run("new images wait in the queue until checked, exam files and pending uploads never do", func(t *testing.T) {
		ownerID := createTestUser(t, pool)
		image := readyOf(t, repo, ownerID, KindFeed, "image/jpeg")
		exam := readyOf(t, repo, ownerID, KindExam, "application/pdf")
		pending, err := repo.createPending(context.Background(), ownerID, KindChat, objectKey(KindChat, "image/png"), "image/png", 10)
		if err != nil {
			t.Fatal(err)
		}

		ids := listed(t)
		if !slices.Contains(ids, image.ID) {
			t.Errorf("ready image missing from the queue")
		}
		if slices.Contains(ids, exam.ID) || slices.Contains(ids, pending.ID) {
			t.Errorf("exam or pending upload listed")
		}

		for range 2 {
			if rec := api.Put("/media/images/"+image.ID.String()+"/review", moderator, map[string]any{}); rec.Code != http.StatusNoContent {
				t.Fatalf("review: %d %s", rec.Code, rec.Body.String())
			}
		}
		if slices.Contains(listed(t), image.ID) {
			t.Errorf("checked image still listed")
		}
		if rec := api.Put("/media/images/"+exam.ID.String()+"/review", moderator, map[string]any{}); rec.Code != http.StatusNotFound {
			t.Errorf("reviewing an exam file = %d, want 404", rec.Code)
		}
	})

	t.Run("removing an image hides it everywhere and deletes its file", func(t *testing.T) {
		ownerID := createTestUser(t, pool)
		image := readyOf(t, repo, ownerID, KindAvatar, "image/png")

		for range 2 {
			if rec := api.Delete("/media/images/"+image.ID.String(), moderator); rec.Code != http.StatusNoContent {
				t.Fatalf("remove: %d %s", rec.Code, rec.Body.String())
			}
		}
		var visible int
		if err := pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM ready_media WHERE id = $1", image.ID).Scan(&visible); err != nil {
			t.Fatal(err)
		}
		if visible != 0 {
			t.Errorf("removed image still in ready_media")
		}
		if !slices.Contains(storage.deleted, image.ObjectKey) {
			t.Errorf("file %s not deleted", image.ObjectKey)
		}
		if slices.Contains(listed(t), image.ID) {
			t.Errorf("removed image still listed")
		}
		if rec := api.Delete("/media/images/"+uuid.NewString(), moderator); rec.Code != http.StatusNotFound {
			t.Errorf("removing an unknown image = %d, want 404", rec.Code)
		}
	})
}
