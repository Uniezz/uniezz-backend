package study

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

type courseRow struct {
	ID           uuid.UUID
	AuthorID     *uuid.UUID
	Name         string
	Description  string
	Difficulty   int
	UniversityID string
	FacultyID    uuid.UUID
	Faculty      string
	RatingCount  int
	RatingSum    int
	CreatedAt    time.Time
	MyRating     *int
}

type courseFields struct {
	Name        string
	Description string
	Difficulty  int
	FacultyID   uuid.UUID
}

type courseEditFields struct {
	Name        *string
	Description *string
	Difficulty  *int
}

type courseFilter struct {
	ViewerID     uuid.UUID
	UniversityID string
	FacultyID    uuid.UUID
	Words        []string
	After        *pageCursor
	Limit        int
}

type similarCoursesFilter struct {
	ViewerID  uuid.UUID
	FacultyID uuid.UUID
	Name      string
	MinScore  float64
	Limit     int
}

type pageCursor struct {
	CreatedAt time.Time `json:"c"`
	ID        uuid.UUID `json:"i"`
}

type courseRepository struct {
	db database.DB
}

func newCourseRepository(db database.DB) *courseRepository {
	return &courseRepository{db: db}
}

const courseColumns = `c.id, c.author_id, c.name, c.description, c.difficulty, c.university_id, c.faculty_id, f.name,
	c.rating_count, c.rating_sum, c.created_at`

const courseFaculty = ` JOIN supported_faculties f ON f.id = c.faculty_id`

func courseRatingOf(viewer string) string {
	return `(SELECT rating FROM course_ratings WHERE course_id = c.id AND user_id = ` + viewer + `)`
}

func scanCourse(row pgx.Row) (*courseRow, error) {
	var c courseRow
	err := row.Scan(&c.ID, &c.AuthorID, &c.Name, &c.Description, &c.Difficulty, &c.UniversityID,
		&c.FacultyID, &c.Faculty, &c.RatingCount, &c.RatingSum, &c.CreatedAt, &c.MyRating)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *courseRepository) create(ctx context.Context, authorID uuid.UUID, f courseFields) (*courseRow, error) {
	const query = `
		WITH c AS (
			INSERT INTO courses (author_id, name, description, difficulty, university_id, faculty_id)
			SELECT $1, $2, $3, $4, sf.university_id, sf.id
			FROM supported_faculties sf WHERE sf.id = $5
			RETURNING *
		)
		SELECT ` + courseColumns + `, NULL::smallint FROM c` + courseFaculty
	row, err := scanCourse(r.db.Conn(ctx).QueryRow(ctx, query,
		authorID, f.Name, f.Description, f.Difficulty, f.FacultyID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidInput
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return nil, ErrCourseExists
	}
	if err != nil {
		return nil, fmt.Errorf("create course: %w", err)
	}
	return row, nil
}

func (r *courseRepository) get(ctx context.Context, viewerID, id uuid.UUID) (*courseRow, error) {
	var args database.Args
	arg := args.Add
	query := `SELECT ` + courseColumns + `, ` + courseRatingOf(arg(viewerID)) + ` FROM courses c` + courseFaculty + ` WHERE c.id = ` + arg(id)
	row, err := scanCourse(r.db.Conn(ctx).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get course: %w", err)
	}
	return row, nil
}

func (r *courseRepository) list(ctx context.Context, f courseFilter) ([]courseRow, error) {
	var args database.Args
	arg := args.Add

	query := `SELECT ` + courseColumns + `, ` + courseRatingOf(arg(f.ViewerID)) + ` FROM courses c` + courseFaculty

	var where []string
	if f.UniversityID != "" {
		where = append(where, "c.university_id = "+arg(f.UniversityID))
	}
	if f.FacultyID != uuid.Nil {
		where = append(where, "c.faculty_id = "+arg(f.FacultyID))
	}
	for _, word := range f.Words {
		where = append(where, "public.search_text(c.name) LIKE "+database.SearchPattern(arg(word)))
	}
	if f.After != nil {
		where = append(where, "(c.created_at, c.id) < ("+arg(f.After.CreatedAt)+", "+arg(f.After.ID)+")")
	}

	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY c.created_at DESC, c.id DESC LIMIT " + arg(f.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list courses: %w", err)
	}
	defer rows.Close()

	var courses []courseRow
	for rows.Next() {
		course, err := scanCourse(rows)
		if err != nil {
			return nil, fmt.Errorf("scan course: %w", err)
		}
		courses = append(courses, *course)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list courses: %w", err)
	}
	return courses, nil
}

func (r *courseRepository) hasSimilar(ctx context.Context, facultyID uuid.UUID, name string, minScore float64) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM courses
			WHERE faculty_id = $1
				AND public.search_text(name) <> public.search_text($2)
				AND public.similarity(public.search_text(name), public.search_text($2)) >= $3
		)`
	var found bool
	if err := r.db.Conn(ctx).QueryRow(ctx, query, facultyID, name, minScore).Scan(&found); err != nil {
		return false, fmt.Errorf("find similar courses: %w", err)
	}
	return found, nil
}

func (r *courseRepository) similar(ctx context.Context, f similarCoursesFilter) ([]courseRow, error) {
	var args database.Args
	arg := args.Add
	score := "public.similarity(public.search_text(c.name), public.search_text(" + arg(f.Name) + "))"
	query := `SELECT ` + courseColumns + `, ` + courseRatingOf(arg(f.ViewerID)) + ` FROM courses c` + courseFaculty + `
		WHERE c.faculty_id = ` + arg(f.FacultyID) + ` AND ` + score + ` >= ` + arg(f.MinScore) + `
		ORDER BY ` + score + ` DESC, c.name, c.id
		LIMIT ` + arg(f.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list similar courses: %w", err)
	}
	defer rows.Close()

	var courses []courseRow
	for rows.Next() {
		course, err := scanCourse(rows)
		if err != nil {
			return nil, fmt.Errorf("scan course: %w", err)
		}
		courses = append(courses, *course)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list similar courses: %w", err)
	}
	return courses, nil
}

func (r *courseRepository) update(ctx context.Context, id, authorID uuid.UUID, f courseEditFields) (*courseRow, error) {
	query := `
		WITH c AS (
			UPDATE courses SET
				name = COALESCE($3, name),
				description = COALESCE($4, description),
				difficulty = COALESCE($5, difficulty)
			WHERE id = $1 AND author_id = $2
			RETURNING *
		)
		SELECT ` + courseColumns + `, ` + courseRatingOf("$2") + ` FROM c` + courseFaculty
	row, err := scanCourse(r.db.Conn(ctx).QueryRow(ctx, query, id, authorID, f.Name, f.Description, f.Difficulty))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return nil, ErrCourseExists
	}
	if err != nil {
		return nil, fmt.Errorf("update course: %w", err)
	}
	return row, nil
}

func (r *courseRepository) lock(ctx context.Context, id uuid.UUID) error {
	const query = `SELECT id FROM courses WHERE id = $1 FOR NO KEY UPDATE`
	var found uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx, query, id).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock course: %w", err)
	}
	return nil
}

func (r *courseRepository) lockForDelete(ctx context.Context, id, authorID uuid.UUID, anyAuthor bool) (bool, error) {
	const query = `SELECT id FROM courses WHERE id = $1 AND ($3 OR author_id = $2) FOR UPDATE`
	var found uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx, query, id, authorID, anyAuthor).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock course: %w", err)
	}
	return true, nil
}

func (r *courseRepository) delete(ctx context.Context, id uuid.UUID) error {
	const query = `DELETE FROM courses WHERE id = $1`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, id); err != nil {
		return fmt.Errorf("delete course: %w", err)
	}
	return nil
}

func (r *courseRepository) getRating(ctx context.Context, courseID, userID uuid.UUID) (int, bool, error) {
	const query = `SELECT rating FROM course_ratings WHERE course_id = $1 AND user_id = $2`
	var rating int
	err := r.db.Conn(ctx).QueryRow(ctx, query, courseID, userID).Scan(&rating)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("get course rating: %w", err)
	}
	return rating, true, nil
}

func (r *courseRepository) saveRating(ctx context.Context, courseID, userID uuid.UUID, rating int) error {
	const query = `
		INSERT INTO course_ratings (course_id, user_id, rating) VALUES ($1, $2, $3)
		ON CONFLICT (course_id, user_id) DO UPDATE SET rating = EXCLUDED.rating`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, courseID, userID, rating); err != nil {
		return fmt.Errorf("save course rating: %w", err)
	}
	return nil
}

func (r *courseRepository) deleteRating(ctx context.Context, courseID, userID uuid.UUID) (int, bool, error) {
	const query = `DELETE FROM course_ratings WHERE course_id = $1 AND user_id = $2 RETURNING rating`
	var rating int
	err := r.db.Conn(ctx).QueryRow(ctx, query, courseID, userID).Scan(&rating)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("delete course rating: %w", err)
	}
	return rating, true, nil
}

func (r *courseRepository) addToRating(ctx context.Context, id uuid.UUID, count, sum int) error {
	const query = `UPDATE courses SET rating_count = rating_count + $2, rating_sum = rating_sum + $3 WHERE id = $1`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, id, count, sum); err != nil {
		return fmt.Errorf("update course rating: %w", err)
	}
	return nil
}

func (r *courseRepository) ratedBy(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	const query = `SELECT course_id FROM course_ratings WHERE user_id = $1 ORDER BY course_id`
	rows, err := r.db.Conn(ctx).Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list rated courses: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, fmt.Errorf("list rated courses: %w", err)
	}
	return ids, nil
}

func (r *courseRepository) chatCards(ctx context.Context, ids []uuid.UUID) ([]ChatCard, error) {
	const query = `SELECT c.id, c.name, f.name FROM courses c` + courseFaculty + ` WHERE c.id = ANY($1)`
	return scanChatCards(ctx, r.db, "list course cards", query, ids)
}
