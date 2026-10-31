package feed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type requestKeyRow struct {
	Fingerprint []byte
	ResourceID  uuid.UUID
}

type requestKeyRepository struct {
	db database.DB
}

func newRequestKeyRepository(db database.DB) *requestKeyRepository {
	return &requestKeyRepository{db: db}
}

func (r *requestKeyRepository) get(ctx context.Context, userID uuid.UUID, operation, key string) (*requestKeyRow, error) {
	var row requestKeyRow
	err := r.db.Conn(ctx).QueryRow(ctx, `
		SELECT fingerprint, resource_id FROM feed_request_keys
		WHERE user_id = $1 AND operation = $2 AND key = $3`,
		userID, operation, key).Scan(&row.Fingerprint, &row.ResourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get request key: %w", err)
	}
	return &row, nil
}

func (r *requestKeyRepository) insert(ctx context.Context, userID uuid.UUID, operation, key string, fingerprint []byte, resourceID uuid.UUID) (bool, error) {
	result, err := r.db.Conn(ctx).Exec(ctx, `
		INSERT INTO feed_request_keys (user_id, operation, key, fingerprint, resource_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, operation, key) DO NOTHING`,
		userID, operation, key, fingerprint, resourceID)
	if err != nil {
		return false, fmt.Errorf("insert request key: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

func (r *requestKeyRepository) pruneOlderThan(ctx context.Context, age time.Duration) error {
	if _, err := r.db.Conn(ctx).Exec(ctx, `DELETE FROM feed_request_keys WHERE created_at < NOW() - $1::interval`, age); err != nil {
		return fmt.Errorf("prune request keys: %w", err)
	}
	return nil
}
