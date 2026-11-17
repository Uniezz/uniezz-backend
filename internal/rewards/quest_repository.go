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

func (r *questRepository) today(ctx context.Context, userID uuid.UUID, zone string) (*questRow, time.Time, error) {
	const query = `
		SELECT d.day, (d.day + 1)::timestamp AT TIME ZONE $2,
			COALESCE(q.likes, 0), COALESCE(q.replied, FALSE), COALESCE(q.posted, FALSE), q.completed_at
		FROM (SELECT (NOW() AT TIME ZONE $2)::date AS day) d
		LEFT JOIN daily_quests q ON q.user_id = $1 AND q.day = d.day`

	var q questRow
	var resetsAt time.Time
	err := r.db.Conn(ctx).QueryRow(ctx, query, userID, zone).Scan(&q.Day, &resetsAt, &q.Likes, &q.Replied, &q.Posted, &q.CompletedAt)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("get today's quests: %w", err)
	}
	return &q, resetsAt, nil
}

func (r *questRepository) deleteOlderThan(ctx context.Context, zone string, days int) error {
	const query = `DELETE FROM daily_quests WHERE day < (NOW() AT TIME ZONE $1)::date - $2::int`

	if _, err := r.db.Conn(ctx).Exec(ctx, query, zone, days); err != nil {
		return fmt.Errorf("delete old quests: %w", err)
	}
	return nil
}

func scanQuest(row pgx.Row) (*questRow, error) {
	var q questRow
	if err := row.Scan(&q.ID, &q.Day, &q.Likes, &q.Replied, &q.Posted, &q.CompletedAt); err != nil {
		return nil, err
	}
	return &q, nil
}
