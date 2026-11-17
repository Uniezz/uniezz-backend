package rewards

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type grantRow struct {
	ID        uuid.UUID
	Reason    Reason
	Amount    int
	CreatedAt time.Time
}

type grantCursor struct {
	CreatedAt time.Time `json:"c"`
	ID        uuid.UUID `json:"i"`
}

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

func (r *coinRepository) listGrants(ctx context.Context, userID uuid.UUID, after *grantCursor, limit int) ([]grantRow, error) {
	args := database.Args{userID}
	arg := args.Add

	where := ""
	if after != nil {
		where = " AND (created_at, id) < (" + arg(after.CreatedAt) + ", " + arg(after.ID) + ")"
	}
	query := `
		SELECT id, reason, amount, created_at
		FROM coin_grants
		WHERE user_id = $1` + where + `
		ORDER BY created_at DESC, id DESC
		LIMIT ` + arg(limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list coin grants: %w", err)
	}
	defer rows.Close()

	var grants []grantRow
	for rows.Next() {
		var g grantRow
		if err := rows.Scan(&g.ID, &g.Reason, &g.Amount, &g.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan coin grant: %w", err)
		}
		grants = append(grants, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list coin grants: %w", err)
	}
	return grants, nil
}
