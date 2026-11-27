package media

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type mediaRow struct {
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

type imageCursor struct {
	CreatedAt time.Time `json:"c"`
	ID        uuid.UUID `json:"i"`
}

type imageFilter struct {
	Kinds []Kind
	After *imageCursor
	Limit int
}

const (
	statusReady   = "ready"
	statusRemoved = "removed"
)

type repository struct {
	db database.DB
}

func newRepository(db database.DB) *repository {
	return &repository{db: db}
}

func (r *repository) createPending(ctx context.Context, ownerUserID uuid.UUID, kind Kind, objectKey, contentType string, sizeBytes int64) (*mediaRow, error) {
	const query = `
        INSERT INTO media (owner_user_id, kind, object_key, content_type, size_bytes)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING ` + mediaColumns

	item, err := scanMedia(r.db.Conn(ctx).QueryRow(ctx, query, ownerUserID, kind, objectKey, contentType, sizeBytes))
	if err != nil {
		return nil, fmt.Errorf("create pending media: %w", err)
	}
	return item, nil
}

func (r *repository) getOwnedPending(ctx context.Context, id, ownerUserID uuid.UUID, kind Kind) (*mediaRow, error) {
	const query = `
        SELECT ` + mediaColumns + `
        FROM media
        WHERE id = $1 AND owner_user_id = $2 AND kind = $3 AND status = 'pending'`

	item, err := scanMedia(r.db.Conn(ctx).QueryRow(ctx, query, id, ownerUserID, kind))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get pending media: %w", err)
	}
	return item, nil
}

func (r *repository) markReady(ctx context.Context, id, ownerUserID uuid.UUID, kind Kind) error {
	const query = `
        UPDATE media
        SET status = 'ready'
        WHERE id = $1 AND owner_user_id = $2 AND kind = $3 AND status = 'pending'`

	result, err := r.db.Conn(ctx).Exec(ctx, query, id, ownerUserID, kind)
	if err != nil {
		return fmt.Errorf("mark media ready: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repository) getReady(ctx context.Context, id uuid.UUID) (*mediaRow, error) {
	const query = `
        SELECT ` + mediaColumns + `
        FROM media
        WHERE id = $1 AND status = 'ready'`

	item, err := scanMedia(r.db.Conn(ctx).QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get ready media: %w", err)
	}
	return item, nil
}

func (r *repository) markDeleting(ctx context.Context, id uuid.UUID) error {
	const query = `UPDATE media SET status = 'deleting' WHERE id = $1 AND status = 'ready'`

	result, err := r.db.Conn(ctx).Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("mark media deleting: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repository) markManyDeleting(ctx context.Context, ids []uuid.UUID) error {
	const query = `UPDATE media SET status = 'deleting' WHERE id = ANY($1) AND status = 'ready'`

	if _, err := r.db.Conn(ctx).Exec(ctx, query, ids); err != nil {
		return fmt.Errorf("mark media deleting: %w", err)
	}
	return nil
}

func (r *repository) getDeleting(ctx context.Context, id uuid.UUID) (*mediaRow, error) {
	const query = `
        SELECT ` + mediaColumns + `
        FROM media
        WHERE id = $1 AND status = 'deleting'`

	item, err := scanMedia(r.db.Conn(ctx).QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get deleting media: %w", err)
	}
	return item, nil
}

func (r *repository) deleteDeleting(ctx context.Context, id uuid.UUID) error {
	if _, err := r.db.Conn(ctx).Exec(ctx, `DELETE FROM media WHERE id = $1 AND status = 'deleting'`, id); err != nil {
		return fmt.Errorf("delete media: %w", err)
	}
	return nil
}

func (r *repository) markAbandonedDeleting(ctx context.Context, olderThan time.Duration) error {
	const query = `
		UPDATE media SET status = 'deleting'
		WHERE status = 'pending' AND created_at < NOW() - $1::interval`

	if _, err := r.db.Conn(ctx).Exec(ctx, query, olderThan); err != nil {
		return fmt.Errorf("mark abandoned uploads deleting: %w", err)
	}
	return nil
}

func (r *repository) listDeleting(ctx context.Context, limit int) ([]uuid.UUID, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, `SELECT id FROM media WHERE status = 'deleting' ORDER BY created_at LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list deleting media: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, fmt.Errorf("list deleting media: %w", err)
	}
	return ids, nil
}

func scanMedia(row pgx.Row) (*mediaRow, error) {
	var item mediaRow
	err := row.Scan(&item.ID, &item.OwnerUserID, &item.Kind, &item.ObjectKey, &item.ContentType, &item.SizeBytes, &item.Status, &item.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *repository) markOwnerDeleting(ctx context.Context, ownerUserID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, `UPDATE media SET status = 'deleting' WHERE owner_user_id = $1 RETURNING id`, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("mark owner media deleting: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan owner media: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mark owner media deleting: %w", err)
	}
	return ids, nil
}

func (r *repository) listUnreviewed(ctx context.Context, f imageFilter) ([]mediaRow, error) {
	var args database.Args
	arg := args.Add

	where := []string{"reviewed_at IS NULL", "status = " + arg(statusReady), "kind = ANY(" + arg(f.Kinds) + ")"}
	if f.After != nil {
		where = append(where, "(created_at, id) > ("+arg(f.After.CreatedAt)+", "+arg(f.After.ID)+")")
	}
	query := `
        SELECT ` + mediaColumns + `
        FROM media
        WHERE ` + strings.Join(where, " AND ") + `
        ORDER BY created_at, id
        LIMIT ` + arg(f.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list unreviewed media: %w", err)
	}
	defer rows.Close()

	var items []mediaRow
	for rows.Next() {
		item, err := scanMedia(rows)
		if err != nil {
			return nil, fmt.Errorf("scan media: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list unreviewed media: %w", err)
	}
	return items, nil
}

func (r *repository) markReviewed(ctx context.Context, id uuid.UUID, kinds []Kind) error {
	const query = `
        UPDATE media SET reviewed_at = COALESCE(reviewed_at, NOW())
        WHERE id = $1 AND status = $2 AND kind = ANY($3)`

	result, err := r.db.Conn(ctx).Exec(ctx, query, id, statusReady, kinds)
	if err != nil {
		return fmt.Errorf("mark media reviewed: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repository) markRemoved(ctx context.Context, id uuid.UUID, kinds []Kind) (string, error) {
	const query = `
        UPDATE media SET status = $2, reviewed_at = COALESCE(reviewed_at, NOW())
        WHERE id = $1 AND status IN ($2, $3) AND kind = ANY($4)
        RETURNING object_key`

	var objectKey string
	err := r.db.Conn(ctx).QueryRow(ctx, query, id, statusRemoved, statusReady, kinds).Scan(&objectKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("mark media removed: %w", err)
	}
	return objectKey, nil
}
