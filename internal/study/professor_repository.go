package study

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type professorRow struct {
	ID           uuid.UUID
	Name         string
	Degree       *string
	UniversityID string
	RatingCount  int
	RatingSum    int
	MyRating     *int
}

type professorFields struct {
	ExternalID   uuid.UUID
	RadonID      string
	Name         string
	Degree       *string
	UniversityID string
}

type professorFilter struct {
	ViewerID     uuid.UUID
	UniversityID string
	Words        []string
	After        *nameCursor
	Limit        int
}

type nameCursor struct {
	Name string    `json:"n"`
	ID   uuid.UUID `json:"i"`
}

type professorRepository struct {
	db database.DB
}

func newProfessorRepository(db database.DB) *professorRepository {
	return &professorRepository{db: db}
}

const professorColumns = `id, name, degree, university_id, rating_count, rating_sum`

func professorRatingOf(viewer string) string {
	return `(SELECT rating FROM professor_ratings WHERE professor_id = professors.id AND user_id = ` + viewer + `)`
}

func scanProfessor(row pgx.Row) (*professorRow, error) {
	var p professorRow
	if err := row.Scan(&p.ID, &p.Name, &p.Degree, &p.UniversityID, &p.RatingCount, &p.RatingSum, &p.MyRating); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *professorRepository) list(ctx context.Context, f professorFilter) ([]professorRow, error) {
	var args database.Args
	arg := args.Add

	columns := professorColumns + `, ` + professorRatingOf(arg(f.ViewerID))
	where := []string{"active"}
	if f.UniversityID != "" {
		where = append(where, "university_id = "+arg(f.UniversityID))
	}
	for _, word := range f.Words {
		where = append(where, "public.search_text(name) LIKE "+database.SearchPattern(arg(word)))
	}
	if f.After != nil {
		where = append(where, "(name, id) > ("+arg(f.After.Name)+", "+arg(f.After.ID)+")")
	}
	query := `SELECT ` + columns + ` FROM professors WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY name, id LIMIT ` + arg(f.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list professors: %w", err)
	}
	defer rows.Close()

	var professors []professorRow
	for rows.Next() {
		professor, err := scanProfessor(rows)
		if err != nil {
			return nil, fmt.Errorf("scan professor: %w", err)
		}
		professors = append(professors, *professor)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list professors: %w", err)
	}
	return professors, nil
}

func (r *professorRepository) get(ctx context.Context, viewerID, id uuid.UUID) (*professorRow, error) {
	var args database.Args
	arg := args.Add
	query := `SELECT ` + professorColumns + `, ` + professorRatingOf(arg(viewerID)) + ` FROM professors WHERE id = ` + arg(id) + ` AND active`
	row, err := scanProfessor(r.db.Conn(ctx).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get professor: %w", err)
	}
	return row, nil
}

func (r *professorRepository) lockActive(ctx context.Context, id uuid.UUID) error {
	const query = `SELECT id FROM professors WHERE id = $1 AND active FOR NO KEY UPDATE`
	var found uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx, query, id).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock professor: %w", err)
	}
	return nil
}

func (r *professorRepository) lock(ctx context.Context, id uuid.UUID) error {
	const query = `SELECT id FROM professors WHERE id = $1 FOR NO KEY UPDATE`
	var found uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx, query, id).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock professor: %w", err)
	}
	return nil
}

func (r *professorRepository) getRating(ctx context.Context, professorID, userID uuid.UUID) (int, bool, error) {
	const query = `SELECT rating FROM professor_ratings WHERE professor_id = $1 AND user_id = $2`
	var rating int
	err := r.db.Conn(ctx).QueryRow(ctx, query, professorID, userID).Scan(&rating)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("get professor rating: %w", err)
	}
	return rating, true, nil
}

func (r *professorRepository) saveRating(ctx context.Context, professorID, userID uuid.UUID, rating int) error {
	const query = `
		INSERT INTO professor_ratings (professor_id, user_id, rating) VALUES ($1, $2, $3)
		ON CONFLICT (professor_id, user_id) DO UPDATE SET rating = EXCLUDED.rating`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, professorID, userID, rating); err != nil {
		return fmt.Errorf("save professor rating: %w", err)
	}
	return nil
}

func (r *professorRepository) deleteRating(ctx context.Context, professorID, userID uuid.UUID) (int, bool, error) {
	const query = `DELETE FROM professor_ratings WHERE professor_id = $1 AND user_id = $2 RETURNING rating`
	var rating int
	err := r.db.Conn(ctx).QueryRow(ctx, query, professorID, userID).Scan(&rating)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("delete professor rating: %w", err)
	}
	return rating, true, nil
}

func (r *professorRepository) addToRating(ctx context.Context, id uuid.UUID, count, sum int) error {
	const query = `UPDATE professors SET rating_count = rating_count + $2, rating_sum = rating_sum + $3 WHERE id = $1`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, id, count, sum); err != nil {
		return fmt.Errorf("update professor rating: %w", err)
	}
	return nil
}

func (r *professorRepository) ratedBy(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	const query = `SELECT professor_id FROM professor_ratings WHERE user_id = $1 ORDER BY professor_id`
	rows, err := r.db.Conn(ctx).Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list rated professors: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, fmt.Errorf("list rated professors: %w", err)
	}
	return ids, nil
}

func (r *professorRepository) countRetiring(ctx context.Context, universityID string, keep []uuid.UUID) (active, retiring int, err error) {
	const query = `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE NOT (external_id = ANY($2)))
		FROM professors WHERE university_id = $1 AND active`
	if err := r.db.Conn(ctx).QueryRow(ctx, query, universityID, keep).Scan(&active, &retiring); err != nil {
		return 0, 0, fmt.Errorf("count retiring professors: %w", err)
	}
	return active, retiring, nil
}

func (r *professorRepository) upsertMany(ctx context.Context, people []professorFields) error {
	externalIDs := make([]uuid.UUID, len(people))
	radonIDs := make([]string, len(people))
	names := make([]string, len(people))
	degrees := make([]*string, len(people))
	universities := make([]string, len(people))
	for i, p := range people {
		externalIDs[i], radonIDs[i], names[i], degrees[i], universities[i] = p.ExternalID, p.RadonID, p.Name, p.Degree, p.UniversityID
	}

	const query = `
		INSERT INTO professors (external_id, radon_id, name, degree, university_id)
		SELECT * FROM unnest($1::uuid[], $2::text[], $3::text[], $4::text[], $5::text[])
		ON CONFLICT (external_id) DO UPDATE SET
			radon_id = EXCLUDED.radon_id,
			name = EXCLUDED.name,
			degree = EXCLUDED.degree,
			university_id = EXCLUDED.university_id,
			active = TRUE
		WHERE (professors.radon_id, professors.name, professors.degree, professors.university_id, professors.active)
			IS DISTINCT FROM (EXCLUDED.radon_id, EXCLUDED.name, EXCLUDED.degree, EXCLUDED.university_id, TRUE)`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, externalIDs, radonIDs, names, degrees, universities); err != nil {
		return fmt.Errorf("upsert professors: %w", err)
	}
	return nil
}

func (r *professorRepository) retire(ctx context.Context, universityID string, keep []uuid.UUID) (int64, error) {
	const query = `
		UPDATE professors SET active = FALSE
		WHERE university_id = $1 AND active AND NOT (external_id = ANY($2))`
	tag, err := r.db.Conn(ctx).Exec(ctx, query, universityID, keep)
	if err != nil {
		return 0, fmt.Errorf("retire professors: %w", err)
	}
	return tag.RowsAffected(), nil
}
