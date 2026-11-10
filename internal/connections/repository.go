package connections

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type repository struct {
	db database.DB
}

func newRepository(db database.DB) *repository {
	return &repository{db: db}
}

type preferences struct {
	BirthDate *time.Time
	OpenTo    []string
	Intent    *string
	SetIntent bool
}

func (r *repository) interests(ctx context.Context, userID uuid.UUID) ([]string, error) {
	var interests []string
	err := r.db.Conn(ctx).QueryRow(ctx, `SELECT COALESCE(interests, ARRAY[]::text[]) FROM users WHERE id = $1`, userID).Scan(&interests)
	if err != nil {
		return nil, fmt.Errorf("load interests: %w", err)
	}
	return interests, nil
}

func (r *repository) savePreferences(ctx context.Context, userID uuid.UUID, p preferences) error {
	_, err := r.db.Conn(ctx).Exec(ctx, `
		UPDATE users
		SET birth_date = COALESCE($2, birth_date),
			open_to = COALESCE($3, open_to),
			intent = CASE WHEN $4 THEN $5 ELSE intent END
		WHERE id = $1`, userID, p.BirthDate, p.OpenTo, p.SetIntent, p.Intent)
	if err != nil {
		return fmt.Errorf("save connection preferences: %w", err)
	}
	return nil
}

type discoverQuery struct {
	Viewer     uuid.UUID
	Mode       string
	University []string
	Faculty    string
	Levels     []string
	Gender     string
	Shared     bool
	Limit      int
}

type cardRow struct {
	UserID      uuid.UUID
	Intent      *string
	Bio         *string
	Interests   []string
	University  string
	Level       *string
	Age         *int
	SharedScore int
}

func (r *repository) discover(ctx context.Context, q discoverQuery) ([]cardRow, error) {
	args := database.Args{q.Viewer, q.Mode}
	where := `
		u.id <> $1
		AND $2 = ANY(u.open_to)
		AND (SELECT $2 = ANY(open_to) FROM users WHERE id = $1)
		AND NOT EXISTS (
			SELECT 1 FROM connection_actions a
			WHERE a.viewer_id = $1 AND a.target_id = u.id AND a.action = 'like')
		AND NOT EXISTS (
			SELECT 1 FROM connection_actions a
			WHERE a.viewer_id = $1 AND a.target_id = u.id AND a.action = 'hide')
		AND NOT EXISTS (
			SELECT 1 FROM connection_actions a
			WHERE a.viewer_id = $1 AND a.target_id = u.id AND a.action = 'pass'
			  AND a.created_at > NOW() - INTERVAL '30 days')
		AND NOT EXISTS (
			SELECT 1 FROM connections c
			WHERE c.user_low = LEAST($1, u.id)::uuid AND c.user_high = GREATEST($1, u.id)::uuid)
		AND NOT EXISTS (
			SELECT 1 FROM blocks b
			WHERE (b.blocker_id = $1 AND b.blocked_id = u.id)
			   OR (b.blocker_id = u.id AND b.blocked_id = $1))`
	if len(q.University) > 0 {
		where += " AND u.university_id = ANY(" + args.Add(q.University) + ")"
	}
	if q.Gender != "" {
		where += " AND u.gender = " + args.Add(q.Gender)
	}
	if q.Mode == modeStudy && q.Faculty != "" {
		where += " AND sp.faculty_name = " + args.Add(q.Faculty)
	}
	if q.Mode == modeStudy && len(q.Levels) > 0 {
		where += " AND sp.level = ANY(" + args.Add(q.Levels) + ")"
	}
	if q.Shared {
		where += " AND u.interests && (SELECT interests FROM users WHERE id = $1)"
	}
	score := `(CASE WHEN sp.programme_name IS NOT NULL AND sp.programme_name = viewer.programme_name THEN 2 ELSE 0 END
		+ CASE WHEN sp.faculty_name IS NOT NULL AND sp.faculty_name = viewer.faculty_name THEN 1 ELSE 0 END
		+ CASE WHEN u.interests && COALESCE((SELECT interests FROM users WHERE id = $1), ARRAY[]::text[]) THEN 1 ELSE 0 END)`
	query := `
		SELECT u.id, u.intent, u.bio, u.interests, u.university_id, sp.level,
			CASE WHEN u.birth_date IS NULL THEN NULL
				ELSE EXTRACT(YEAR FROM age(CURRENT_DATE, u.birth_date))::int END, ` + score + `
		FROM users u
		LEFT JOIN student_programmes sp ON sp.user_id = u.id AND sp.is_primary
		LEFT JOIN student_programmes viewer ON viewer.user_id = $1 AND viewer.is_primary
		WHERE ` + where + `
		ORDER BY ` + score + ` DESC, u.id
		LIMIT ` + args.Add(q.Limit)
	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}
	defer rows.Close()
	var cards []cardRow
	for rows.Next() {
		var card cardRow
		if err := rows.Scan(&card.UserID, &card.Intent, &card.Bio, &card.Interests, &card.University, &card.Level, &card.Age, &card.SharedScore); err != nil {
			return nil, fmt.Errorf("scan discovery card: %w", err)
		}
		cards = append(cards, card)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}
	return cards, nil
}

func (r *repository) candidate(ctx context.Context, viewer, userID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx, `
		INSERT INTO discovery_candidates (viewer_id, user_id) VALUES ($1, $2)
		ON CONFLICT (viewer_id, user_id) DO UPDATE SET viewer_id = EXCLUDED.viewer_id
		RETURNING id`, viewer, userID).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("save candidate: %w", err)
	}
	return id, nil
}

func (r *repository) lookup(ctx context.Context, viewer, candidate uuid.UUID) (uuid.UUID, error) {
	var userID uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx, `
		SELECT user_id FROM discovery_candidates WHERE id = $1 AND viewer_id = $2`, candidate, viewer).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("lookup candidate: %w", err)
	}
	return userID, nil
}

func (r *repository) act(ctx context.Context, viewer, target uuid.UUID, mode, action string) error {
	_, err := r.db.Conn(ctx).Exec(ctx, `
		INSERT INTO connection_actions (viewer_id, target_id, mode, action)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING`, viewer, target, mode, action)
	if err != nil {
		return fmt.Errorf("save connection action: %w", err)
	}
	return nil
}

func (r *repository) liked(ctx context.Context, viewer, target uuid.UUID, mode string) (bool, error) {
	var found bool
	err := r.db.Conn(ctx).QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM connection_actions
			WHERE viewer_id = $1 AND target_id = $2 AND mode = $3 AND action = 'like'
		)`, viewer, target, mode).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("check like: %w", err)
	}
	return found, nil
}

type connectionRow struct {
	ID        uuid.UUID
	OtherID   uuid.UUID
	Mode      string
	CreatedAt time.Time
}

func (r *repository) match(ctx context.Context, self, otherID uuid.UUID, mode string) (*connectionRow, bool, error) {
	var row connectionRow
	err := r.db.Conn(ctx).QueryRow(ctx, `
		INSERT INTO connections (user_low, user_high, mode)
		VALUES (LEAST($1, $2)::uuid, GREATEST($1, $2)::uuid, $3)
		ON CONFLICT (user_low, user_high) DO NOTHING
		RETURNING id, CASE WHEN user_low = $1::uuid THEN user_high ELSE user_low END, mode, created_at`,
		self, otherID, mode).Scan(&row.ID, &row.OtherID, &row.Mode, &row.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = r.db.Conn(ctx).QueryRow(ctx, `
			SELECT id, CASE WHEN user_low = $1::uuid THEN user_high ELSE user_low END, mode, created_at
			FROM connections
			WHERE user_low = LEAST($1, $2)::uuid AND user_high = GREATEST($1, $2)::uuid`, self, otherID).Scan(&row.ID, &row.OtherID, &row.Mode, &row.CreatedAt)
		if err != nil {
			return nil, false, fmt.Errorf("get connection: %w", err)
		}
		return &row, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("create connection: %w", err)
	}
	return &row, true, nil
}

func (r *repository) list(ctx context.Context, userID uuid.UUID, after *pageCursor, limit int) ([]connectionRow, error) {
	args := database.Args{userID}
	where := "(c.user_low = $1 OR c.user_high = $1)"
	if after != nil {
		where += " AND (c.created_at, c.id) < (" + args.Add(after.CreatedAt) + ", " + args.Add(after.ID) + ")"
	}
	query := `
		SELECT c.id, CASE WHEN c.user_low = $1 THEN c.user_high ELSE c.user_low END, c.mode, c.created_at
		FROM connections c
		WHERE ` + where + `
		  AND NOT EXISTS (
			SELECT 1 FROM blocks b
			WHERE (b.blocker_id = $1 AND b.blocked_id = CASE WHEN c.user_low = $1 THEN c.user_high ELSE c.user_low END)
			   OR (b.blocked_id = $1 AND b.blocker_id = CASE WHEN c.user_low = $1 THEN c.user_high ELSE c.user_low END))
		ORDER BY c.created_at DESC, c.id DESC
		LIMIT ` + args.Add(limit)
	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	defer rows.Close()
	var connections []connectionRow
	for rows.Next() {
		var row connectionRow
		if err := rows.Scan(&row.ID, &row.OtherID, &row.Mode, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan connection: %w", err)
		}
		connections = append(connections, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	return connections, nil
}

func (r *repository) delete(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := r.db.Conn(ctx).Exec(ctx, `
		DELETE FROM connections
		WHERE id = $1 AND (user_low = $2 OR user_high = $2)`, id, userID)
	if err != nil {
		return fmt.Errorf("delete connection: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repository) block(ctx context.Context, blocker, blocked uuid.UUID) error {
	if blocker == blocked {
		return ErrInvalidInput
	}
	_, err := r.db.Conn(ctx).Exec(ctx, `
		INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, blocker, blocked)
	if err != nil {
		return fmt.Errorf("block: %w", err)
	}
	return nil
}

func (r *repository) unblock(ctx context.Context, blocker, blocked uuid.UUID) error {
	_, err := r.db.Conn(ctx).Exec(ctx, `DELETE FROM blocks WHERE blocker_id = $1 AND blocked_id = $2`, blocker, blocked)
	if err != nil {
		return fmt.Errorf("unblock: %w", err)
	}
	return nil
}

func (r *repository) card(ctx context.Context, userID uuid.UUID) (cardRow, error) {
	var card cardRow
	err := r.db.Conn(ctx).QueryRow(ctx, `
		SELECT u.id, u.intent, u.bio, u.interests, u.university_id, sp.level,
			CASE WHEN u.birth_date IS NULL THEN NULL
				ELSE EXTRACT(YEAR FROM age(CURRENT_DATE, u.birth_date))::int END, 0
		FROM users u
		LEFT JOIN student_programmes sp ON sp.user_id = u.id AND sp.is_primary
		WHERE u.id = $1`, userID).Scan(&card.UserID, &card.Intent, &card.Bio, &card.Interests, &card.University, &card.Level, &card.Age, &card.SharedScore)
	if errors.Is(err, pgx.ErrNoRows) {
		return card, ErrNotFound
	}
	if err != nil {
		return card, fmt.Errorf("load card: %w", err)
	}
	return card, nil
}
