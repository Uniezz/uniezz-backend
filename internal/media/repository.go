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

const mediaColumns = "id, owner_user_id, kind, object_key, content_type, size_bytes, status, created_at"

type repository struct {
	db *pgxpool.Pool
}

func newRepository(db *pgxpool.Pool) *repository {
	return &repository{db: db}
}

func (r *repository) createPending(ctx context.Context, ownerUserID uuid.UUID, kind Kind, objectKey, contentType string, sizeBytes int64) (*Media, error) {
	const query = `
        INSERT INTO media (owner_user_id, kind, object_key, content_type, size_bytes)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING ` + mediaColumns

	item, err := scanMedia(r.db.QueryRow(ctx, query, ownerUserID, kind, objectKey, contentType, sizeBytes))
	if err != nil {
		return nil, fmt.Errorf("create pending media: %w", err)
	}
	return item, nil
}

func (r *repository) getOwnedPending(ctx context.Context, id, ownerUserID uuid.UUID, kind Kind) (*Media, error) {
	const query = `
        SELECT ` + mediaColumns + `
        FROM media
        WHERE id = $1 AND owner_user_id = $2 AND kind = $3 AND status = 'pending'`

	item, err := scanMedia(r.db.QueryRow(ctx, query, id, ownerUserID, kind))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get pending media: %w", err)
	}
	return item, nil
}

func (r *repository) markReady(ctx context.Context, id, ownerUserID uuid.UUID) error {
	const query = `
        UPDATE media
        SET status = 'ready'
        WHERE id = $1 AND owner_user_id = $2 AND status = 'pending'`

	result, err := r.db.Exec(ctx, query, id, ownerUserID)
	if err != nil {
		return fmt.Errorf("mark media ready: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repository) getReady(ctx context.Context, id uuid.UUID) (*Media, error) {
	const query = `
        SELECT ` + mediaColumns + `
        FROM media
        WHERE id = $1 AND status = 'ready'`

	item, err := scanMedia(r.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get ready media: %w", err)
	}
	return item, nil
}

func scanMedia(row pgx.Row) (*Media, error) {
	var item Media
	err := row.Scan(&item.ID, &item.OwnerUserID, &item.Kind, &item.ObjectKey, &item.ContentType, &item.SizeBytes, &item.Status, &item.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &item, nil
}
