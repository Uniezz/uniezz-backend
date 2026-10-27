package media

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	storage *Storage
	repo    *repository
}

type Upload struct {
	MediaID   uuid.UUID
	UploadURL string
	ExpiresAt time.Time
}

func New(ctx context.Context, db *pgxpool.Pool, cfg *config.Config) (Module, error) {
	storage, err := newStorage(ctx, cfg)
	if err != nil {
		return Module{}, err
	}

	if err := storage.CheckBucket(ctx); err != nil {
		slog.WarnContext(ctx, "media storage bucket is unavailable", "error", err)
	}

	return Module{
		storage: storage,
		repo:    newRepository(db),
	}, nil
}

func (m Module) Claim(
	ctx context.Context,
	mediaID uuid.UUID,
	ownerUserID uuid.UUID,
	kind Kind,
) error {
	return claimUploadedMedia(ctx, m.repo, m.storage, mediaID, ownerUserID, kind)
}

func (m Module) URL(ctx context.Context, mediaID uuid.UUID) (string, error) {
	item, err := m.repo.getReady(ctx, mediaID)
	if err != nil {
		return "", err
	}

	switch item.Kind {
	case KindAvatar, KindFeed:
		return m.storage.PublicURL(item.ObjectKey)
	case KindChat:
		return m.storage.PresignGetURL(ctx, item.ObjectKey)
	default:
		return "", fmt.Errorf("unsupported media kind %q", item.Kind)
	}
}

func (m Module) CreateUpload(
	ctx context.Context,
	ownerUserID uuid.UUID,
	kind Kind,
	contentType string,
	size int64,
) (Upload, error) {
	if err := validateUpload(kind, contentType, size); err != nil {
		return Upload{}, err
	}

	key, err := NewObjectKey(kind, contentType)
	if err != nil {
		return Upload{}, err
	}

	uploadURL, err := m.storage.PresignPutURL(ctx, key, kind, contentType, size)
	if err != nil {
		return Upload{}, fmt.Errorf("create media upload URL: %w", err)
	}

	item, err := m.repo.createPending(ctx, ownerUserID, kind, key, contentType, size)
	if err != nil {
		return Upload{}, err
	}

	return Upload{
		MediaID:   item.ID,
		UploadURL: uploadURL,
		ExpiresAt: time.Now().Add(uploadURLTTL),
	}, nil
}
