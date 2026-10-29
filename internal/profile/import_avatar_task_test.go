package profile

import (
	"context"
	"errors"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/google/uuid"
)

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
