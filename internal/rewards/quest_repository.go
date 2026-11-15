package rewards

import (
	"context"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type questStep struct {
	Likes   int
	Replied bool
	Posted  bool
}

type questRow struct {
	ID          uuid.UUID
	Day         time.Time
	Likes       int
	Replied     bool
	Posted      bool
	CompletedAt *time.Time
}

const questColumns = "id, day, likes, replied, posted, completed_at"

type questRepository struct {
	db database.DB
}

func newQuestRepository(db database.DB) *questRepository {
	return &questRepository{db: db}
}

func (r *questRepository) record(ctx context.Context, userID uuid.UUID, zone string, step questStep) (*questRow, error) {
	const query = `
		INSERT INTO daily_quests (user_id, day, likes, replied, posted)
		VALUES ($1, (NOW() AT TIME ZONE $2)::date, $3, $4, $5)
		ON CONFLICT (user_id, day) DO UPDATE SET
			likes = daily_quests.likes + EXCLUDED.likes,
			replied = daily_quests.replied OR EXCLUDED.replied,
			posted = daily_quests.posted OR EXCLUDED.posted
		RETURNING ` + questColumns

	row, err := scanQuest(r.db.Conn(ctx).QueryRow(ctx, query, userID, zone, step.Likes, step.Replied, step.Posted))
	if err != nil {
		return nil, fmt.Errorf("record quest: %w", err)
	}
	return row, nil
}

func (r *questRepository) complete(ctx context.Context, id uuid.UUID) (bool, error) {
	result, err := r.db.Conn(ctx).Exec(ctx, `UPDATE daily_quests SET completed_at = NOW() WHERE id = $1 AND completed_at IS NULL`, id)
	if err != nil {
		return false, fmt.Errorf("complete quests: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

func scanQuest(row pgx.Row) (*questRow, error) {
	var q questRow
	if err := row.Scan(&q.ID, &q.Day, &q.Likes, &q.Replied, &q.Posted, &q.CompletedAt); err != nil {
		return nil, err
	}
	return &q, nil
}
