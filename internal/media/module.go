// Package media stores user uploads such as avatars, feed images and chat attachments. Clients upload files straight to S3-compatible storage through presigned URLs; other modules keep only a media ID and ask this package for a URL to show.
package media

import (
	"context"
	"log/slog"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ mediaStore = (*repository)(nil)

type mediaStore interface {
	createPending(ctx context.Context, ownerUserID uuid.UUID, kind Kind, objectKey, contentType string, sizeBytes int64) (*Media, error)
	getOwnedPending(ctx context.Context, id, ownerUserID uuid.UUID, kind Kind) (*Media, error)
	markReady(ctx context.Context, id, ownerUserID uuid.UUID) error
	getReady(ctx context.Context, id uuid.UUID) (*Media, error)
}

var _ objectStore = (*objectStorage)(nil)

type objectStore interface {
	headObject(ctx context.Context, key string) (size int64, contentType string, err error)
	presignPut(ctx context.Context, key, contentType string, size int64) (string, error)
	presignGet(ctx context.Context, key string) (string, error)
	publicURL(key string) (string, error)
}

type Module struct {
	storage objectStore
	repo    mediaStore
}

func New(ctx context.Context, db *pgxpool.Pool, cfg *config.Config) (Module, error) {
	storage, err := newObjectStorage(ctx, cfg)
	if err != nil {
		return Module{}, err
	}

	if err := storage.checkBucket(ctx); err != nil {
		slog.WarnContext(ctx, "media storage bucket is unavailable", "error", err)
	}

	return Module{storage: storage, repo: newRepository(db)}, nil
}
