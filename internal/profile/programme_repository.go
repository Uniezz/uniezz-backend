package profile

import (
	"context"
	"errors"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type programmeRow struct {
	ID            uuid.UUID
	ProgrammeID   *uuid.UUID
	Faculty       *string
	Programme     string
	Level         *string
	ModeOfStudies *string
	YearOfStudy   *int
	Status        string
	IsPrimary     bool
}

type programmeFields struct {
	ProgrammeID   uuid.UUID
	ModeOfStudies *string
	YearOfStudy   *int
	Status        string
	IsPrimary     bool
}

type programmeRepository struct {
	db database.DB
}

func newProgrammeRepository(db database.DB) *programmeRepository {
	return &programmeRepository{db: db}
}

const programmeColumns = `id, programme_id, faculty_name, programme_name, level, mode_of_studies, year_of_study, status, is_primary`

func scanProgramme(row pgx.Row) (*programmeRow, error) {
	var p programmeRow
	if err := row.Scan(&p.ID, &p.ProgrammeID, &p.Faculty, &p.Programme, &p.Level, &p.ModeOfStudies, &p.YearOfStudy, &p.Status, &p.IsPrimary); err != nil {
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
		if err := r.lockUser(ctx, userID); err != nil {
			return err
		}
		if f.IsPrimary {
			if err := r.clearPrimary(ctx, userID, uuid.Nil); err != nil {
				return err
			}
		}

		var id uuid.UUID
		err := r.db.Conn(ctx).QueryRow(ctx, `
			INSERT INTO student_programmes (user_id, programme_id, faculty_name, programme_name, level, mode_of_studies, year_of_study, status, is_primary)
			SELECT $1, p.id, p.faculty_name, p.name, p.level, $3, $4, $5, $6
			FROM supported_programmes p JOIN users u ON u.university_id = p.university_id
			WHERE p.id = $2 AND u.id = $1
			RETURNING id`,
			userID, f.ProgrammeID, f.ModeOfStudies, f.YearOfStudy, f.Status, f.IsPrimary).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidInput
		}
		if err != nil {
			return fmt.Errorf("create programme: %w", err)
		}
		if err := r.ensurePrimary(ctx, userID); err != nil {
			return err
		}

		created, err = r.get(ctx, id)
		return err
	})
	return created, err
}

func (r *programmeRepository) replace(ctx context.Context, userID, id uuid.UUID, f programmeFields) (*programmeRow, error) {
	var updated *programmeRow
	err := r.db.WithTx(ctx, func(ctx context.Context) error {
		if err := r.lockUser(ctx, userID); err != nil {
			return err
		}

		var wasPrimary bool
		err := r.db.Conn(ctx).QueryRow(ctx, `
            SELECT COALESCE(
                (SELECT is_primary FROM student_programmes WHERE id = $1 AND user_id = $2),
                FALSE
            )`, id, userID).Scan(&wasPrimary)
		if err != nil {
			return fmt.Errorf("get programme primary status: %w", err)
		}

		if f.IsPrimary {
			if err := r.clearPrimary(ctx, userID, id); err != nil {
				return err
			}
		}

		var catalogued bool
		err = r.db.Conn(ctx).QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM supported_programmes p JOIN users u ON u.university_id = p.university_id
				WHERE p.id = $1 AND u.id = $2
			)`, f.ProgrammeID, userID).Scan(&catalogued)
		if err != nil {
			return fmt.Errorf("check catalogue programme: %w", err)
		}
		if !catalogued {
			return ErrInvalidInput
		}

		result, err := r.db.Conn(ctx).Exec(ctx, `
            UPDATE student_programmes sp
            SET programme_id = p.id, faculty_name = p.faculty_name, programme_name = p.name, level = p.level,
                mode_of_studies = $4, year_of_study = $5, status = $6, is_primary = $7
            FROM supported_programmes p
            WHERE sp.id = $1 AND sp.user_id = $2 AND p.id = $3`,
			id, userID, f.ProgrammeID, f.ModeOfStudies, f.YearOfStudy, f.Status, f.IsPrimary)
		if err != nil {
			return fmt.Errorf("replace programme: %w", err)
		}
		if result.RowsAffected() == 0 {
			return ErrNotFound
		}

		if wasPrimary && !f.IsPrimary {
			if err := r.promoteOldestOther(ctx, userID, id); err != nil {
				return err
			}
		}
		if err := r.ensurePrimary(ctx, userID); err != nil {
			return err
		}

		updated, err = r.get(ctx, id)
		return err
	})
	return updated, err
}

func (r *programmeRepository) promoteOldestOther(ctx context.Context, userID, exceptID uuid.UUID) error {
	_, err := r.db.Conn(ctx).Exec(ctx, `
        UPDATE student_programmes
        SET is_primary = TRUE
        WHERE id = (
            SELECT id
            FROM student_programmes
            WHERE user_id = $1 AND id <> $2
            ORDER BY created_at, id
            LIMIT 1
        )`, userID, exceptID)
	if err != nil {
		return fmt.Errorf("promote another primary programme: %w", err)
	}
	return nil
}

func (r *programmeRepository) delete(ctx context.Context, userID, id uuid.UUID) error {
	return r.db.WithTx(ctx, func(ctx context.Context) error {
		if err := r.lockUser(ctx, userID); err != nil {
			return err
		}

		result, err := r.db.Conn(ctx).Exec(ctx, `DELETE FROM student_programmes WHERE id = $1 AND user_id = $2`, id, userID)
		if err != nil {
			return fmt.Errorf("delete programme: %w", err)
		}
		if result.RowsAffected() == 0 {
			return ErrNotFound
		}
		return r.ensurePrimary(ctx, userID)
	})
}

func (r *programmeRepository) insertInitial(ctx context.Context, userID uuid.UUID, programmes []auth.InitialProgramme) error {
	for _, p := range programmes {
		_, err := r.db.Conn(ctx).Exec(ctx, `
			INSERT INTO student_programmes (user_id, programme_id, faculty_name, programme_name, level, mode_of_studies, is_primary)
			VALUES ($1, (
				SELECT p.id FROM supported_programmes p JOIN users u ON u.university_id = p.university_id
				WHERE u.id = $1 AND public.search_text(p.name) = public.search_text($3::text)
					AND ($4::text IS NULL OR p.level = $4::text)
				ORDER BY p.active DESC, p.level, p.id
				LIMIT 1
			), $2, $3, $4, $5, $6)`,
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

func (r *programmeRepository) get(ctx context.Context, id uuid.UUID) (*programmeRow, error) {
	p, err := scanProgramme(r.db.Conn(ctx).QueryRow(ctx, `SELECT `+programmeColumns+` FROM student_programmes WHERE id = $1`, id))
	if err != nil {
		return nil, fmt.Errorf("get programme: %w", err)
	}
	return p, nil
}

func (r *programmeRepository) lockUser(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.db.Conn(ctx).Exec(ctx, `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, userID); err != nil {
		return fmt.Errorf("lock user programmes: %w", err)
	}
	return nil
}

func (r *programmeRepository) ensurePrimary(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.Conn(ctx).Exec(ctx, `
		UPDATE student_programmes
		SET is_primary = TRUE
		WHERE id = (
			SELECT id
			FROM student_programmes
			WHERE user_id = $1
			ORDER BY created_at, id
			LIMIT 1)
		AND NOT EXISTS (
			SELECT 1
			FROM student_programmes
			WHERE user_id = $1 AND is_primary)`, userID)
	if err != nil {
		return fmt.Errorf("ensure primary programme: %w", err)
	}
	return nil
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
