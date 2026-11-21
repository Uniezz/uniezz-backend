package rewards

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const checkViolation = "23514"

// onceStart is the period of a challenge that is done once: every action counts towards one row.
var onceStart = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

type challengeRow struct {
	ID         uuid.UUID
	Action     Action
	Target     int
	Period     Period
	Reward     int
	Texts      map[string]ChallengeText
	StartsAt   *time.Time
	EndsAt     *time.Time
	ArchivedAt *time.Time
	CreatedAt  time.Time
}

type myChallengeRow struct {
	challengeRow
	Progress  int
	Completed bool
	ResetsAt  *time.Time
}

type challengeFields struct {
	Action   Action
	Target   int
	Period   Period
	Reward   int
	Texts    map[string]ChallengeText
	StartsAt *time.Time
	EndsAt   *time.Time
}

type challengeUpdate struct {
	Texts    map[string]ChallengeText
	Reward   *int
	StartsAt *time.Time
	EndsAt   *time.Time
}

type challengeCursor struct {
	CreatedAt time.Time `json:"c"`
	ID        uuid.UUID `json:"i"`
}

type challengeFilter struct {
	UserID uuid.UUID
	After  *challengeCursor
	Limit  int
}

type completedChallenge struct {
	ProgressID uuid.UUID
	Reward     int
}

const challengeColumns = "c.id, c.action, c.target, c.period, c.reward, c.texts, c.starts_at, c.ends_at, c.archived_at, c.created_at"

func scanChallenge(row pgx.Row, extra ...any) (challengeRow, error) {
	var c challengeRow
	dest := append([]any{&c.ID, &c.Action, &c.Target, &c.Period, &c.Reward, &c.Texts, &c.StartsAt, &c.EndsAt, &c.ArchivedAt, &c.CreatedAt}, extra...)
	err := row.Scan(dest...)
	return c, err
}

// periodStart is the first day of the challenge's current period in the quest zone.
func periodStart(arg func(any) string, zone string) string {
	local := "(NOW() AT TIME ZONE " + arg(zone) + ")"
	return "CASE c.period WHEN " + arg(PeriodDay) + " THEN " + local + "::date" +
		" WHEN " + arg(PeriodWeek) + " THEN date_trunc('week', " + local + ")::date" +
		" ELSE " + arg(onceStart) + "::date END"
}

func activeChallenge() string {
	return "c.archived_at IS NULL AND (c.starts_at IS NULL OR c.starts_at <= NOW()) AND (c.ends_at IS NULL OR c.ends_at > NOW())"
}

type challengeRepository struct {
	db database.DB
}

func newChallengeRepository(db database.DB) *challengeRepository {
	return &challengeRepository{db: db}
}

func (r *challengeRepository) create(ctx context.Context, f challengeFields) (*challengeRow, error) {
	const query = `
		INSERT INTO challenges AS c (action, target, period, reward, texts, starts_at, ends_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING ` + challengeColumns
	row, err := scanChallenge(r.db.Conn(ctx).QueryRow(ctx, query, f.Action, f.Target, f.Period, f.Reward, f.Texts, f.StartsAt, f.EndsAt))
	if err != nil {
		return nil, mapChallengeError("create challenge", err)
	}
	return &row, nil
}

func (r *challengeRepository) get(ctx context.Context, id uuid.UUID) (*challengeRow, error) {
	const query = `SELECT ` + challengeColumns + ` FROM challenges c WHERE c.id = $1`
	row, err := scanChallenge(r.db.Conn(ctx).QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get challenge: %w", err)
	}
	return &row, nil
}

func (r *challengeRepository) update(ctx context.Context, id uuid.UUID, u challengeUpdate) (*challengeRow, error) {
	const query = `
		UPDATE challenges c SET
			texts = COALESCE($2, c.texts),
			reward = COALESCE($3, c.reward),
			starts_at = COALESCE($4, c.starts_at),
			ends_at = COALESCE($5, c.ends_at)
		WHERE c.id = $1
		RETURNING ` + challengeColumns
	var texts any
	if u.Texts != nil {
		texts = u.Texts
	}
	row, err := scanChallenge(r.db.Conn(ctx).QueryRow(ctx, query, id, texts, u.Reward, u.StartsAt, u.EndsAt))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, mapChallengeError("update challenge", err)
	}
	return &row, nil
}

func (r *challengeRepository) archive(ctx context.Context, id uuid.UUID) error {
	const query = `UPDATE challenges SET archived_at = COALESCE(archived_at, NOW()) WHERE id = $1`
	result, err := r.db.Conn(ctx).Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("archive challenge: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *challengeRepository) list(ctx context.Context, f challengeFilter) ([]challengeRow, error) {
	var args database.Args
	arg := args.Add

	var where []string
	if f.After != nil {
		where = append(where, "(c.created_at, c.id) < ("+arg(f.After.CreatedAt)+", "+arg(f.After.ID)+")")
	}
	query := `SELECT ` + challengeColumns + ` FROM challenges c`
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, " AND ")
	}
	query += ` ORDER BY c.created_at DESC, c.id DESC LIMIT ` + arg(f.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list challenges: %w", err)
	}
	defer rows.Close()

	var challenges []challengeRow
	for rows.Next() {
		row, err := scanChallenge(rows)
		if err != nil {
			return nil, fmt.Errorf("scan challenge: %w", err)
		}
		challenges = append(challenges, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list challenges: %w", err)
	}
	return challenges, nil
}

func (r *challengeRepository) listMine(ctx context.Context, zone string, f challengeFilter) ([]myChallengeRow, error) {
	var args database.Args
	arg := args.Add

	start := periodStart(arg, zone)
	where := []string{activeChallenge()}
	if f.After != nil {
		where = append(where, "(c.created_at, c.id) < ("+arg(f.After.CreatedAt)+", "+arg(f.After.ID)+")")
	}
	query := `
		SELECT ` + challengeColumns + `, COALESCE(p.progress, 0), p.completed_at IS NOT NULL,
			CASE c.period
				WHEN ` + arg(PeriodDay) + ` THEN (ps.start + 1)::timestamp AT TIME ZONE ` + arg(zone) + `
				WHEN ` + arg(PeriodWeek) + ` THEN (ps.start + 7)::timestamp AT TIME ZONE ` + arg(zone) + `
			END
		FROM challenges c
		CROSS JOIN LATERAL (SELECT ` + start + ` AS start) ps
		LEFT JOIN challenge_progress p ON p.user_id = ` + arg(f.UserID) + ` AND p.challenge_id = c.id AND p.period_start = ps.start
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY c.created_at DESC, c.id DESC
		LIMIT ` + arg(f.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list my challenges: %w", err)
	}
	defer rows.Close()

	var challenges []myChallengeRow
	for rows.Next() {
		var mine myChallengeRow
		row, err := scanChallenge(rows, &mine.Progress, &mine.Completed, &mine.ResetsAt)
		if err != nil {
			return nil, fmt.Errorf("scan my challenge: %w", err)
		}
		mine.challengeRow = row
		challenges = append(challenges, mine)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list my challenges: %w", err)
	}
	return challenges, nil
}

// step counts one action towards every active challenge for it and returns the challenges it
// completed. The upsert's row lock orders concurrent actions, so each completes a period once.
func (r *challengeRepository) step(ctx context.Context, userID uuid.UUID, action Action, zone string) ([]completedChallenge, error) {
	var args database.Args
	arg := args.Add

	query := `
		WITH active AS (
			SELECT c.id, c.target, c.reward, ` + periodStart(arg, zone) + ` AS start
			FROM challenges c
			WHERE c.action = ` + arg(action) + ` AND ` + activeChallenge() + `
		),
		stepped AS (
			INSERT INTO challenge_progress AS p (user_id, challenge_id, period_start, target, progress, completed_at)
			SELECT ` + arg(userID) + `, a.id, a.start, a.target, 1, CASE WHEN a.target <= 1 THEN NOW() END
			FROM active a
			ON CONFLICT (user_id, challenge_id, period_start) DO UPDATE SET
				progress = p.progress + 1,
				completed_at = CASE WHEN p.progress + 1 >= p.target THEN NOW() END
			WHERE p.completed_at IS NULL
			RETURNING p.id, p.challenge_id, p.completed_at IS NOT NULL AS completed
		)
		SELECT s.id, a.reward FROM stepped s JOIN active a ON a.id = s.challenge_id WHERE s.completed`

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("step challenges: %w", err)
	}
	defer rows.Close()

	var completed []completedChallenge
	for rows.Next() {
		var c completedChallenge
		if err := rows.Scan(&c.ProgressID, &c.Reward); err != nil {
			return nil, fmt.Errorf("scan completed challenge: %w", err)
		}
		completed = append(completed, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("step challenges: %w", err)
	}
	return completed, nil
}

func (r *challengeRepository) deleteOldProgress(ctx context.Context, zone string, days int) error {
	const query = `
		DELETE FROM challenge_progress
		WHERE period_start <> $3 AND period_start < (NOW() AT TIME ZONE $1)::date - $2::int`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, zone, days, onceStart); err != nil {
		return fmt.Errorf("delete old challenge progress: %w", err)
	}
	return nil
}

func mapChallengeError(op string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == checkViolation {
		return fmt.Errorf("%w: %s", ErrInvalidInput, pgErr.ConstraintName)
	}
	return fmt.Errorf("%s: %w", op, err)
}
