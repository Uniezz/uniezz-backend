package profile

import (
	"context"
	"errors"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/database"
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
	db database.DB
}

func newProgrammeRepository(db *pgxpool.Pool) *programmeRepository {
	return &programmeRepository{db: database.New(db)}
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
	rows, err := r.db.Conn(ctx).Query(ctx, `
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
	err := r.db.WithTx(ctx, func(ctx context.Context) error {
		if f.IsPrimary {
			if err := r.clearPrimary(ctx, userID, uuid.Nil); err != nil {
				return err
			}
		}

		var err error
		created, err = scanProgramme(r.db.Conn(ctx).QueryRow(ctx, `
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
	err := r.db.WithTx(ctx, func(ctx context.Context) error {
		if f.IsPrimary {
			if err := r.clearPrimary(ctx, userID, id); err != nil {
				return err
			}
		}

		var err error
		updated, err = scanProgramme(r.db.Conn(ctx).QueryRow(ctx, `
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
	result, err := r.db.Conn(ctx).Exec(ctx, `DELETE FROM student_programmes WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete programme: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *programmeRepository) insertInitial(ctx context.Context, userID uuid.UUID, programmes []auth.InitialProgramme) error {
	for _, p := range programmes {
		_, err := r.db.Conn(ctx).Exec(ctx, `
			INSERT INTO student_programmes (user_id, faculty_name, programme_name, level, mode_of_studies, is_primary)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			userID, p.Faculty, p.Programme, nullIfEmpty(p.Level), nullIfEmpty(p.ModeOfStudies), p.IsPrimary)
		if err != nil {
			return fmt.Errorf("insert initial programme: %w", err)
		}
	}
	return nil
}

func (r *programmeRepository) clearPrimary(ctx context.Context, userID, except uuid.UUID) error {
	_, err := r.db.Conn(ctx).Exec(ctx, `UPDATE student_programmes SET is_primary = FALSE WHERE user_id = $1 AND is_primary AND id <> $2`, userID, except)
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
