package media

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Media struct {
	ID          uuid.UUID
	OwnerUserID uuid.UUID
	Kind        Kind
	ObjectKey   string
	ContentType string
	SizeBytes   int64
	Status      string
	CreatedAt   time.Time
}

type repository struct {
	db *pgxpool.Pool
}

func newRepository(db *pgxpool.Pool) *repository {
	return &repository{db: db}
}

func (r *repository) createPending(
	ctx context.Context,
	ownerUserID uuid.UUID,
	kind Kind,
	objectKey string,
	contentType string,
	sizeBytes int64,
) (*Media, error) {
	const query = `
        INSERT INTO media (owner_user_id, kind, object_key, content_type, size_bytes)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING id, owner_user_id, kind, object_key, content_type, size_bytes, status, created_at;
    `

	var item Media
	err := r.db.QueryRow(ctx, query, ownerUserID, kind, objectKey, contentType, sizeBytes).Scan(
		&item.ID,
		&item.OwnerUserID,
		&item.Kind,
		&item.ObjectKey,
		&item.ContentType,
		&item.SizeBytes,
		&item.Status,
		&item.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create pending media: %w", err)
	}

	return &item, nil
}

func (r *repository) getOwnedPending(
	ctx context.Context,
	id uuid.UUID,
	ownerUserID uuid.UUID,
	kind Kind,
) (*Media, error) {
	const query = `
        SELECT id, owner_user_id, kind, object_key, content_type, size_bytes, status, created_at
        FROM media
        WHERE id = $1 AND owner_user_id = $2 AND kind = $3 AND status = 'pending';
    `

	var item Media
	err := r.db.QueryRow(ctx, query, id, ownerUserID, kind).Scan(
		&item.ID,
		&item.OwnerUserID,
		&item.Kind,
		&item.ObjectKey,
		&item.ContentType,
		&item.SizeBytes,
		&item.Status,
		&item.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("pending media not found: %w", err)
		}
		return nil, fmt.Errorf("get pending media: %w", err)
	}

	return &item, nil
}

func (r *repository) markReady(
	ctx context.Context,
	id uuid.UUID,
	ownerUserID uuid.UUID,
) error {
	const query = `
        UPDATE media
        SET status = 'ready'
        WHERE id = $1 AND owner_user_id = $2 AND status = 'pending';
    `

	result, err := r.db.Exec(ctx, query, id, ownerUserID)
	if err != nil {
		return fmt.Errorf("mark media ready: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("pending media not found: %w", pgx.ErrNoRows)
	}

	return nil
}

func (r *repository) getReady(ctx context.Context, id uuid.UUID) (*Media, error) {
	const query = `
        SELECT id, owner_user_id, kind, object_key, content_type, size_bytes, status, created_at
        FROM media
        WHERE id = $1 AND status = 'ready';
    `

	var item Media
	err := r.db.QueryRow(ctx, query, id).Scan(
		&item.ID,
		&item.OwnerUserID,
		&item.Kind,
		&item.ObjectKey,
		&item.ContentType,
		&item.SizeBytes,
		&item.Status,
		&item.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("ready media not found: %w", err)
		}
		return nil, fmt.Errorf("get ready media: %w", err)
	}

	return &item, nil
}
