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

const commentCourseForeignKey = "course_comments_course_id_fkey"

type commentRow struct {
	ID        uuid.UUID
	CourseID  uuid.UUID
	AuthorID  uuid.UUID
	Content   string
	CreatedAt time.Time
}

type commentFilter struct {
	ViewerID uuid.UUID
	CourseID uuid.UUID
	After    *pageCursor
	Limit    int
}

type commentRepository struct {
	db database.DB
}

func newCommentRepository(db database.DB) *commentRepository {
	return &commentRepository{db: db}
}

const commentColumns = `c.id, c.course_id, c.author_id, c.content, c.created_at`

func scanComment(row pgx.Row) (*commentRow, error) {
	var c commentRow
	if err := row.Scan(&c.ID, &c.CourseID, &c.AuthorID, &c.Content, &c.CreatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *commentRepository) create(ctx context.Context, courseID, authorID uuid.UUID, content string) (*commentRow, error) {
	const query = `
		INSERT INTO course_comments AS c (course_id, author_id, content)
		VALUES ($1, $2, $3)
		RETURNING ` + commentColumns
	row, err := scanComment(r.db.Conn(ctx).QueryRow(ctx, query, courseID, authorID, content))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation && pgErr.ConstraintName == commentCourseForeignKey {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("create course comment: %w", err)
	}
	return row, nil
}

func (r *commentRepository) list(ctx context.Context, f commentFilter) ([]commentRow, error) {
	var args database.Args
	arg := args.Add

	query := `SELECT ` + commentColumns + ` FROM course_comments c
		WHERE c.course_id = ` + arg(f.CourseID) + ` AND ` + notBlocked(arg(f.ViewerID), "c.author_id")
	if f.After != nil {
		query += " AND (c.created_at, c.id) > (" + arg(f.After.CreatedAt) + ", " + arg(f.After.ID) + ")"
	}
	query += " ORDER BY c.created_at, c.id LIMIT " + arg(f.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list course comments: %w", err)
	}
	defer rows.Close()

	var comments []commentRow
	for rows.Next() {
		comment, err := scanComment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan course comment: %w", err)
		}
		comments = append(comments, *comment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list course comments: %w", err)
	}
	return comments, nil
}

func (r *commentRepository) delete(ctx context.Context, id, authorID uuid.UUID) error {
	const query = `DELETE FROM course_comments WHERE id = $1 AND author_id = $2`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, id, authorID); err != nil {
		return fmt.Errorf("delete course comment: %w", err)
	}
	return nil
}

func (r *commentRepository) deleteByCourse(ctx context.Context, courseID uuid.UUID) error {
	const query = `DELETE FROM course_comments WHERE course_id = $1`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, courseID); err != nil {
		return fmt.Errorf("delete course comments by course: %w", err)
	}
	return nil
}

func (r *commentRepository) deleteByAuthor(ctx context.Context, authorID uuid.UUID) error {
	const query = `DELETE FROM course_comments WHERE author_id = $1`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, authorID); err != nil {
		return fmt.Errorf("delete course comments by author: %w", err)
	}
	return nil
}
