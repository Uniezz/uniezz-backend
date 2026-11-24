package study

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const professorReviewProfessorForeignKey = "professor_reviews_professor_id_fkey"

type professorReviewRow struct {
	ID          uuid.UUID
	ProfessorID uuid.UUID
	AuthorID    uuid.UUID
	Content     string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type professorReviewFilter struct {
	ViewerID    uuid.UUID
	ProfessorID uuid.UUID
	After       *pageCursor
	Limit       int
}

type professorReviewRepository struct {
	db database.DB
}

func newProfessorReviewRepository(db database.DB) *professorReviewRepository {
	return &professorReviewRepository{db: db}
}

const professorReviewColumns = `r.id, r.professor_id, r.author_id, r.content, r.created_at, r.updated_at`

func scanProfessorReview(row pgx.Row) (*professorReviewRow, error) {
	var r professorReviewRow
	if err := row.Scan(&r.ID, &r.ProfessorID, &r.AuthorID, &r.Content, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	return &r, nil
}

func (r *professorReviewRepository) save(ctx context.Context, professorID, authorID uuid.UUID, content string) (*professorReviewRow, error) {
	const query = `
		INSERT INTO professor_reviews AS r (professor_id, author_id, content)
		VALUES ($1, $2, $3)
		ON CONFLICT (professor_id, author_id) DO UPDATE SET content = EXCLUDED.content
		RETURNING ` + professorReviewColumns
	row, err := scanProfessorReview(r.db.Conn(ctx).QueryRow(ctx, query, professorID, authorID, content))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation && pgErr.ConstraintName == professorReviewProfessorForeignKey {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("save professor review: %w", err)
	}
	return row, nil
}

func (r *professorReviewRepository) get(ctx context.Context, id uuid.UUID) (*professorReviewRow, error) {
	const query = `SELECT ` + professorReviewColumns + ` FROM professor_reviews r WHERE r.id = $1`
	row, err := scanProfessorReview(r.db.Conn(ctx).QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get professor review: %w", err)
	}
	return row, nil
}

func (r *professorReviewRepository) list(ctx context.Context, f professorReviewFilter) ([]professorReviewRow, error) {
	var args database.Args
	arg := args.Add

	query := `SELECT ` + professorReviewColumns + ` FROM professor_reviews r
		WHERE r.professor_id = ` + arg(f.ProfessorID) + ` AND ` + notHidden(arg(f.ViewerID), "r.author_id")
	if f.After != nil {
		query += " AND (r.created_at, r.id) < (" + arg(f.After.CreatedAt) + ", " + arg(f.After.ID) + ")"
	}
	query += " ORDER BY r.created_at DESC, r.id DESC LIMIT " + arg(f.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list professor reviews: %w", err)
	}
	defer rows.Close()

	var reviews []professorReviewRow
	for rows.Next() {
		review, err := scanProfessorReview(rows)
		if err != nil {
			return nil, fmt.Errorf("scan professor review: %w", err)
		}
		reviews = append(reviews, *review)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list professor reviews: %w", err)
	}
	return reviews, nil
}

func (r *professorReviewRepository) delete(ctx context.Context, professorID, authorID uuid.UUID) error {
	const query = `DELETE FROM professor_reviews WHERE professor_id = $1 AND author_id = $2`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, professorID, authorID); err != nil {
		return fmt.Errorf("delete professor review: %w", err)
	}
	return nil
}

func (r *professorReviewRepository) deleteByAuthor(ctx context.Context, authorID uuid.UUID) error {
	const query = `DELETE FROM professor_reviews WHERE author_id = $1`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, authorID); err != nil {
		return fmt.Errorf("delete professor reviews by author: %w", err)
	}
	return nil
}
