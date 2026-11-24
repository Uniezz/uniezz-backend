package push

import (
	"context"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

type deliveryRepository struct {
	db database.DB
}

func newDeliveryRepository(db database.DB) *deliveryRepository {
	return &deliveryRepository{db: db}
}

func (r *deliveryRepository) claim(ctx context.Context, userIDs []uuid.UUID, collapseID uuid.UUID, window time.Duration) ([]uuid.UUID, error) {
	const query = `
		INSERT INTO push_deliveries (user_id, collapse_id)
		SELECT unnest($1::uuid[]), $2
		ON CONFLICT (user_id, collapse_id) DO UPDATE SET sent_at = NOW()
		WHERE push_deliveries.sent_at < NOW() - $3::interval
		RETURNING user_id`
	rows, err := r.db.Conn(ctx).Query(ctx, query, userIDs, collapseID, window)
	if err != nil {
		return nil, fmt.Errorf("claim push deliveries: %w", err)
	}
	defer rows.Close()

	var allowed []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan push delivery: %w", err)
		}
		allowed = append(allowed, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim push deliveries: %w", err)
	}
	return allowed, nil
}

func (r *deliveryRepository) prune(ctx context.Context, olderThan time.Duration) error {
	const query = `DELETE FROM push_deliveries WHERE sent_at < NOW() - $1::interval`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, olderThan); err != nil {
		return fmt.Errorf("prune push deliveries: %w", err)
	}
	return nil
}
