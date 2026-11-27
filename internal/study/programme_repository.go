package study

import (
	"context"
	"fmt"
	"strings"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

type programmeRow struct {
	ID           uuid.UUID
	UniversityID string
	FacultyID    *uuid.UUID
	Faculty      *string
	Name         string
	Level        ProgrammeLevel
}

type programmeFields struct {
	Name    string
	Level   ProgrammeLevel
	Faculty string
}

type programmeFilter struct {
	UniversityID string
	FacultyID    uuid.UUID
	Words        []string
	After        *nameCursor
	Limit        int
}

type programmeRepository struct {
	db database.DB
}

func newProgrammeRepository(db database.DB) *programmeRepository {
	return &programmeRepository{db: db}
}

func (r *programmeRepository) list(ctx context.Context, f programmeFilter) ([]programmeRow, error) {
	var args database.Args
	arg := args.Add

	where := []string{"p.active", "p.university_id = " + arg(f.UniversityID)}
	if f.FacultyID != uuid.Nil {
		where = append(where, "p.faculty_id = "+arg(f.FacultyID))
	}
	for _, word := range f.Words {
		where = append(where, "public.search_text(p.name) LIKE "+database.SearchPattern(arg(word)))
	}
	if f.After != nil {
		where = append(where, "(p.name, p.id) > ("+arg(f.After.Name)+", "+arg(f.After.ID)+")")
	}
	query := `
		SELECT p.id, p.university_id, p.faculty_id, sf.name, p.name, p.level
		FROM programmes p
		LEFT JOIN supported_faculties sf ON sf.id = p.faculty_id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY p.name, p.id
		LIMIT ` + arg(f.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list programmes: %w", err)
	}
	defer rows.Close()

	var programmes []programmeRow
	for rows.Next() {
		var p programmeRow
		if err := rows.Scan(&p.ID, &p.UniversityID, &p.FacultyID, &p.Faculty, &p.Name, &p.Level); err != nil {
			return nil, fmt.Errorf("scan programme: %w", err)
		}
		programmes = append(programmes, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list programmes: %w", err)
	}
	return programmes, nil
}

func (r *programmeRepository) countRetiring(ctx context.Context, universityID string, keep []programmeFields) (active, retiring int, err error) {
	names, levels := programmeKeys(keep)
	const query = `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE (level, public.search_text(name)) NOT IN (
			SELECT k.level, public.search_text(k.name) FROM unnest($2::text[], $3::text[]) AS k(name, level)
		))
		FROM programmes WHERE university_id = $1 AND active`
	if err := r.db.Conn(ctx).QueryRow(ctx, query, universityID, names, levels).Scan(&active, &retiring); err != nil {
		return 0, 0, fmt.Errorf("count retiring programmes: %w", err)
	}
	return active, retiring, nil
}

func (r *programmeRepository) upsertMany(ctx context.Context, universityID string, programmes []programmeFields) error {
	names, levels := programmeKeys(programmes)
	faculties := make([]string, len(programmes))
	for i, p := range programmes {
		faculties[i] = p.Faculty
	}
	const query = `
		INSERT INTO programmes (university_id, faculty_id, name, level)
		SELECT DISTINCT ON (p.level, public.search_text(p.name))
			$1::text,
			(SELECT sf.id FROM supported_faculties sf
				WHERE sf.university_id = $1::text
					AND regexp_replace(public.search_text(sf.name), '[^a-z0-9]+', '', 'g')
						= regexp_replace(public.search_text(p.faculty), '[^a-z0-9]+', '', 'g')
				ORDER BY sf.position, sf.id LIMIT 1),
			p.name, p.level
		FROM unnest($2::text[], $3::text[], $4::text[]) WITH ORDINALITY AS p(name, level, faculty, n)
		ORDER BY p.level, public.search_text(p.name), p.faculty = '', p.n
		ON CONFLICT (university_id, level, (public.search_text(name))) DO UPDATE SET
			name = EXCLUDED.name,
			faculty_id = EXCLUDED.faculty_id,
			active = TRUE
		WHERE (programmes.name, programmes.faculty_id, programmes.active)
			IS DISTINCT FROM (EXCLUDED.name, EXCLUDED.faculty_id, TRUE)`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, universityID, names, levels, faculties); err != nil {
		return fmt.Errorf("upsert programmes: %w", err)
	}
	return nil
}

func (r *programmeRepository) retire(ctx context.Context, universityID string, keep []programmeFields) (int64, error) {
	names, levels := programmeKeys(keep)
	const query = `
		UPDATE programmes SET active = FALSE
		WHERE university_id = $1 AND active AND (level, public.search_text(name)) NOT IN (
			SELECT k.level, public.search_text(k.name) FROM unnest($2::text[], $3::text[]) AS k(name, level)
		)`
	tag, err := r.db.Conn(ctx).Exec(ctx, query, universityID, names, levels)
	if err != nil {
		return 0, fmt.Errorf("retire programmes: %w", err)
	}
	return tag.RowsAffected(), nil
}

func programmeKeys(programmes []programmeFields) (names, levels []string) {
	names = make([]string, len(programmes))
	levels = make([]string, len(programmes))
	for i, p := range programmes {
		names[i], levels[i] = p.Name, string(p.Level)
	}
	return names, levels
}
