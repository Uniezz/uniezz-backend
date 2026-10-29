package profile

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type user struct {
	ID            uuid.UUID
	UniversityID  string
	Email         *string
	FirstName     *string
	LastName      *string
	AvatarMediaID *uuid.UUID
	CreatedAt     time.Time
}

type programme struct {
	ID            uuid.UUID
	Faculty       string
	Programme     string
	Level         *string
	ModeOfStudies *string
	YearOfStudy   *int
	Status        string
	IsPrimary     bool
}

type programmeFields struct {
	Faculty       string
	Programme     string
	Level         *string
	ModeOfStudies *string
	YearOfStudy   *int
	Status        string
	IsPrimary     bool
}

type repository struct {
	db *pgxpool.Pool
}

func newRepository(db *pgxpool.Pool) *repository {
	return &repository{db: db}
}

func (r *repository) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, r.db, fn)
}

func (r *repository) getUser(ctx context.Context, userID uuid.UUID) (*user, error) {
	const query = `
		SELECT id, university_id, email, first_name, last_name, avatar_media_id, created_at
		FROM users
		WHERE id = $1`

	var u user
	err := r.db.QueryRow(ctx, query, userID).Scan(&u.ID, &u.UniversityID, &u.Email, &u.FirstName, &u.LastName, &u.AvatarMediaID, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get profile: %w", err)
	}
	return &u, nil
}

func (r *repository) updateNames(ctx context.Context, tx pgx.Tx, userID uuid.UUID, firstName, lastName *string) error {
	const query = `
		UPDATE users
		SET first_name = COALESCE($2, first_name), last_name = COALESCE($3, last_name)
		WHERE id = $1`

	if _, err := tx.Exec(ctx, query, userID, firstName, lastName); err != nil {
		return fmt.Errorf("update names: %w", err)
	}
	return nil
}

func (r *repository) replaceAvatar(ctx context.Context, tx pgx.Tx, userID uuid.UUID, mediaID *uuid.UUID) (*uuid.UUID, error) {
	var previous *uuid.UUID
	err := tx.QueryRow(ctx, `SELECT avatar_media_id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&previous)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("lock avatar: %w", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE users SET avatar_media_id = $2 WHERE id = $1`, userID, mediaID); err != nil {
		return nil, fmt.Errorf("set avatar: %w", err)
	}
	return previous, nil
}

func (r *repository) setAvatarIfEmpty(ctx context.Context, userID, mediaID uuid.UUID) (bool, error) {
	result, err := r.db.Exec(ctx, `UPDATE users SET avatar_media_id = $2 WHERE id = $1 AND avatar_media_id IS NULL`, userID, mediaID)
	if err != nil {
		return false, fmt.Errorf("set imported avatar: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

const programmeColumns = `id, faculty_name, programme_name, level, mode_of_studies, year_of_study, status, is_primary`

func scanProgramme(row pgx.Row) (*programme, error) {
	var p programme
	if err := row.Scan(&p.ID, &p.Faculty, &p.Programme, &p.Level, &p.ModeOfStudies, &p.YearOfStudy, &p.Status, &p.IsPrimary); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *repository) listProgrammes(ctx context.Context, userID uuid.UUID) ([]programme, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+programmeColumns+`
		FROM student_programmes
		WHERE user_id = $1
		ORDER BY is_primary DESC, created_at, id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list programmes: %w", err)
	}

	programmes := []programme{}
	for rows.Next() {
		p, err := scanProgramme(rows)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan programme: %w", err)
		}
		programmes = append(programmes, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list programmes: %w", err)
	}
	return programmes, nil
}

func (r *repository) createProgramme(ctx context.Context, userID uuid.UUID, f programmeFields) (*programme, error) {
	var created *programme
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		if f.IsPrimary {
			if err := clearPrimary(ctx, tx, userID, uuid.Nil); err != nil {
				return err
			}
		}

		var err error
		created, err = scanProgramme(tx.QueryRow(ctx, `
			INSERT INTO student_programmes (user_id, faculty_name, programme_name, level, mode_of_studies, year_of_study, status, is_primary)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING `+programmeColumns,
			userID, f.Faculty, f.Programme, f.Level, f.ModeOfStudies, f.YearOfStudy, f.Status, f.IsPrimary))
		if err != nil {
			return fmt.Errorf("create programme: %w", err)
		}
		return nil
	})
	return created, err
}

func (r *repository) replaceProgramme(ctx context.Context, userID, id uuid.UUID, f programmeFields) (*programme, error) {
	var updated *programme
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		if f.IsPrimary {
			if err := clearPrimary(ctx, tx, userID, id); err != nil {
				return err
			}
		}

		var err error
		updated, err = scanProgramme(tx.QueryRow(ctx, `
			UPDATE student_programmes
			SET faculty_name = $3, programme_name = $4, level = $5, mode_of_studies = $6,
				year_of_study = $7, status = $8, is_primary = $9
			WHERE id = $1 AND user_id = $2
			RETURNING `+programmeColumns,
			id, userID, f.Faculty, f.Programme, f.Level, f.ModeOfStudies, f.YearOfStudy, f.Status, f.IsPrimary))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("replace programme: %w", err)
		}
		return nil
	})
	return updated, err
}

func (r *repository) deleteProgramme(ctx context.Context, userID, id uuid.UUID) error {
	result, err := r.db.Exec(ctx, `DELETE FROM student_programmes WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete programme: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repository) insertSeededProgrammes(ctx context.Context, tx pgx.Tx, userID uuid.UUID, seeds []auth.ProgrammeSeed) error {
	for _, s := range seeds {
		_, err := tx.Exec(ctx, `
			INSERT INTO student_programmes (user_id, faculty_name, programme_name, level, mode_of_studies, is_primary)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			userID, s.Faculty, s.Programme, nullIfEmpty(s.Level), nullIfEmpty(s.ModeOfStudies), s.IsPrimary)
		if err != nil {
			return fmt.Errorf("insert seeded programme: %w", err)
		}
	}
	return nil
}

func clearPrimary(ctx context.Context, tx pgx.Tx, userID, except uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE student_programmes SET is_primary = FALSE WHERE user_id = $1 AND is_primary AND id <> $2`, userID, except)
	if err != nil {
		return fmt.Errorf("clear primary programme: %w", err)
	}
	return nil
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
