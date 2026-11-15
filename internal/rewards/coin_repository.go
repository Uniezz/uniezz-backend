package rewards

import (
	"context"
	"errors"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type coinRepository struct {
	db database.DB
}

func newCoinRepository(db database.DB) *coinRepository {
	return &coinRepository{db: db}
}

func (r *coinRepository) addGrant(ctx context.Context, userID uuid.UUID, reason Reason, refID, actorID *uuid.UUID, amount int) (bool, error) {
	const query = `
		INSERT INTO coin_grants (user_id, reason, ref_id, actor_id, amount)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT DO NOTHING`

	result, err := r.db.Conn(ctx).Exec(ctx, query, userID, reason, refID, actorID, amount)
	if err != nil {
		return false, fmt.Errorf("add coin grant: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

func (r *coinRepository) addToBalance(ctx context.Context, userID uuid.UUID, amount int) error {
	const query = `
		INSERT INTO coin_balances (user_id, coins)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET coins = coin_balances.coins + EXCLUDED.coins`

	if _, err := r.db.Conn(ctx).Exec(ctx, query, userID, amount); err != nil {
		return fmt.Errorf("add to coin balance: %w", err)
	}
	return nil
}

func (r *coinRepository) balance(ctx context.Context, userID uuid.UUID) (int, error) {
	var coins int
	err := r.db.Conn(ctx).QueryRow(ctx, `SELECT coins FROM coin_balances WHERE user_id = $1`, userID).Scan(&coins)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get coin balance: %w", err)
	}
	return coins, nil
}
