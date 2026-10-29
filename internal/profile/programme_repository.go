package profile

import (
	"context"
	"errors"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type programmeRow struct {
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

type programmeRepository struct {
	db *pgxpool.Pool
}

func newProgrammeRepository(db *pgxpool.Pool) *programmeRepository {
	return &programmeRepository{db: db}
}

const programmeColumns = `id, faculty_name, programme_name, level, mode_of_studies, year_of_study, status, is_primary`

func scanProgramme(row pgx.Row) (*programmeRow, error) {
	var p programmeRow
	if err := row.Scan(&p.ID, &p.Faculty, &p.Programme, &p.Level, &p.ModeOfStudies, &p.YearOfStudy, &p.Status, &p.IsPrimary); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *programmeRepository) list(ctx context.Context, userID uuid.UUID) ([]programmeRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+programmeColumns+`
		FROM student_programmes
		WHERE user_id = $1
		ORDER BY is_primary DESC, created_at, id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list programmes: %w", err)
	}

	programmes := []programmeRow{}
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

func (r *programmeRepository) create(ctx context.Context, userID uuid.UUID, f programmeFields) (*programmeRow, error) {
	var created *programmeRow
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
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

func (r *programmeRepository) replace(ctx context.Context, userID, id uuid.UUID, f programmeFields) (*programmeRow, error) {
	var updated *programmeRow
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
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

func (r *programmeRepository) delete(ctx context.Context, userID, id uuid.UUID) error {
	result, err := r.db.Exec(ctx, `DELETE FROM student_programmes WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete programme: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *programmeRepository) insertSeeded(ctx context.Context, tx pgx.Tx, userID uuid.UUID, seeds []auth.ProgrammeSeed) error {
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
