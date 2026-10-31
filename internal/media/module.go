// Package media stores user uploads such as avatars, feed images and chat attachments. Clients upload files straight to S3-compatible storage through presigned URLs; other modules keep only a media ID and ask this package for a URL to show.
package media

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ mediaStore = (*repository)(nil)

type mediaStore interface {
	createPending(ctx context.Context, ownerUserID uuid.UUID, kind Kind, objectKey, contentType string, sizeBytes int64) (*mediaRow, error)
	getOwnedPending(ctx context.Context, id, ownerUserID uuid.UUID, kind Kind) (*mediaRow, error)
	markReady(ctx context.Context, id, ownerUserID uuid.UUID) error
	getReady(ctx context.Context, id uuid.UUID) (*mediaRow, error)
	markDeleting(ctx context.Context, tx pgx.Tx, id uuid.UUID) error
	getDeleting(ctx context.Context, id uuid.UUID) (*mediaRow, error)
	deleteDeleting(ctx context.Context, id uuid.UUID) error
}

var _ objectStore = (*objectStorage)(nil)

type objectStore interface {
	headObject(ctx context.Context, key string) (size int64, contentType string, err error)
	presignPut(ctx context.Context, key, contentType string, size int64) (string, error)
	presignGet(ctx context.Context, key string) (string, error)
	publicURL(key string) (string, error)
	putObject(ctx context.Context, key, contentType string, body []byte) error
	deleteObject(ctx context.Context, key string) error
}

type Module struct {
	storage    objectStore
	repo       mediaStore
	httpClient *http.Client
}

func New(ctx context.Context, db *pgxpool.Pool, cfg *config.Config) (Module, error) {
	storage, err := newObjectStorage(ctx, cfg)
	if err != nil {
		return Module{}, err
	}

	if err := storage.checkBucket(ctx); err != nil {
		slog.WarnContext(ctx, "media storage bucket is unavailable", "error", err)
	}

	return Module{storage: storage, repo: newRepository(db), httpClient: &http.Client{Timeout: 10 * time.Second}}, nil
}

func (m Module) Register(api huma.API) {
	uploads := &uploadHandler{media: m}
	huma.Register(api, createUploadOperation, uploads.createUpload)
}
