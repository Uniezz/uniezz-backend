package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type liveFields struct {
	MessageID uuid.UUID
	UserID    uuid.UUID
	Lat       float64
	Lng       float64
	Accuracy  *int
	Heading   *int
	Until     time.Time
}

type liveRow struct {
	MessageID uuid.UUID
	UserID    uuid.UUID
	Lat       float64
	Lng       float64
	Accuracy  *int
	Heading   *int
	UpdatedAt time.Time
	Expired   bool
}

type liveLocationRepository struct {
	db database.DB
}

func newLiveLocationRepository(db database.DB) *liveLocationRepository {
	return &liveLocationRepository{db: db}
}

const liveColumns = "message_id, user_id, lat, lng, accuracy, heading, moved_at, live_until <= NOW()"

func scanLive(row pgx.Row) (*liveRow, error) {
	var l liveRow
	if err := row.Scan(&l.MessageID, &l.UserID, &l.Lat, &l.Lng, &l.Accuracy, &l.Heading, &l.UpdatedAt, &l.Expired); err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *liveLocationRepository) now(ctx context.Context) (time.Time, error) {
	var now time.Time
	if err := r.db.Conn(ctx).QueryRow(ctx, `SELECT NOW()`).Scan(&now); err != nil {
		return time.Time{}, fmt.Errorf("read clock: %w", err)
	}
	return now.UTC(), nil
}

func (r *liveLocationRepository) create(ctx context.Context, f liveFields) error {
	const query = `
		INSERT INTO chat_live_locations (message_id, user_id, lat, lng, live_until)
		VALUES ($1, $2, $3, $4, $5)`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, f.MessageID, f.UserID, f.Lat, f.Lng, f.Until); err != nil {
		return fmt.Errorf("create live location: %w", err)
	}
	return nil
}

func (r *liveLocationRepository) move(ctx context.Context, f liveFields, every time.Duration) (*liveRow, error) {
	const query = `
		UPDATE chat_live_locations
		SET lat = $3, lng = $4, accuracy = $5, heading = $6, moved_at = NOW()
		WHERE message_id = $1 AND user_id = $2 AND live_until > NOW() AND moved_at <= NOW() - $7::interval
		RETURNING ` + liveColumns
	l, err := scanLive(r.db.Conn(ctx).QueryRow(ctx, query, f.MessageID, f.UserID, f.Lat, f.Lng, f.Accuracy, f.Heading, every))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("move live location: %w", err)
	}
	return l, nil
}

func (r *liveLocationRepository) get(ctx context.Context, messageID uuid.UUID) (*liveRow, error) {
	query := `SELECT ` + liveColumns + ` FROM chat_live_locations WHERE message_id = $1`
	l, err := scanLive(r.db.Conn(ctx).QueryRow(ctx, query, messageID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get live location: %w", err)
	}
	return l, nil
}

func (r *liveLocationRepository) getMany(ctx context.Context, messageIDs []uuid.UUID) ([]liveRow, error) {
	query := `SELECT ` + liveColumns + ` FROM chat_live_locations WHERE message_id = ANY($1)`
	rows, err := r.db.Conn(ctx).Query(ctx, query, messageIDs)
	if err != nil {
		return nil, fmt.Errorf("list live locations: %w", err)
	}
	defer rows.Close()

	var lives []liveRow
	for rows.Next() {
		l, err := scanLive(rows)
		if err != nil {
			return nil, fmt.Errorf("scan live location: %w", err)
		}
		lives = append(lives, *l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list live locations: %w", err)
	}
	return lives, nil
}

func (r *liveLocationRepository) finish(ctx context.Context, messageID uuid.UUID) (bool, error) {
	const query = `
		WITH ended AS (
			DELETE FROM chat_live_locations WHERE message_id = $1
			RETURNING message_id, lat, lng, moved_at, live_until
		)
		UPDATE chat_messages msg
		SET payload = jsonb_set(
			jsonb_set(
				msg.payload || jsonb_build_object('lat', ended.lat, 'lng', ended.lng),
				'{live,stoppedAt}', to_jsonb(LEAST(NOW(), ended.live_until))),
			'{live,updatedAt}', to_jsonb(ended.moved_at))
		FROM ended
		WHERE msg.id = ended.message_id`
	result, err := r.db.Conn(ctx).Exec(ctx, query, messageID)
	if err != nil {
		return false, fmt.Errorf("finish live location: %w", err)
	}
	return result.RowsAffected() > 0, nil
}

func (r *liveLocationRepository) delete(ctx context.Context, messageID uuid.UUID) error {
	const query = `DELETE FROM chat_live_locations WHERE message_id = $1`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, messageID); err != nil {
		return fmt.Errorf("delete live location: %w", err)
	}
	return nil
}

func (r *liveLocationRepository) expired(ctx context.Context, limit int) ([]uuid.UUID, error) {
	const query = `
		SELECT message_id FROM chat_live_locations
		WHERE live_until <= NOW()
		ORDER BY live_until
		LIMIT $1`
	return scanUUIDs(ctx, r.db, "list expired live locations", query, limit)
}
