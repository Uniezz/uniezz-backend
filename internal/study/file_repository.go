package study

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	foreignKeyViolation  = "23503"
	fileCourseForeignKey = "course_files_course_id_fkey"
)

type fileRow struct {
	ID        uuid.UUID
	CourseID  uuid.UUID
	AuthorID  uuid.UUID
	Title     string
	Type      FileType
	CreatedAt time.Time
	MediaKind media.Kind
	ObjectKey string
}

type fileFields struct {
	CourseID uuid.UUID
	MediaID  uuid.UUID
	Title    string
	Type     FileType
}

type fileFilter struct {
	ViewerID     uuid.UUID
	CourseID     uuid.UUID
	AuthorID     uuid.UUID
	Words        []string
	UniversityID string
	Faculty      string
	Subject      string
	Semester     int
	After        *pageCursor
	Limit        int
}

type fileRepository struct {
	db database.DB
}

func newFileRepository(db database.DB) *fileRepository {
	return &fileRepository{db: db}
}

const fileColumns = `f.id, f.course_id, f.author_id, f.title, f.file_type, f.created_at, m.kind, m.object_key`

func scanFile(row pgx.Row) (*fileRow, error) {
	var f fileRow
	if err := row.Scan(&f.ID, &f.CourseID, &f.AuthorID, &f.Title, &f.Type, &f.CreatedAt, &f.MediaKind, &f.ObjectKey); err != nil {
		return nil, err
	}
	return &f, nil
}

func notBlocked(viewer, author string) string {
	return "NOT EXISTS (SELECT 1 FROM blocked_pairs b WHERE b.user_id = " + viewer + " AND b.other_id = " + author + ")"
}

func (r *fileRepository) create(ctx context.Context, authorID uuid.UUID, f fileFields) (uuid.UUID, error) {
	const query = `
		INSERT INTO course_files (course_id, media_id, author_id, title, file_type)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`
	var id uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx, query, f.CourseID, f.MediaID, authorID, f.Title, f.Type).Scan(&id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation && pgErr.ConstraintName == fileCourseForeignKey {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("create course file: %w", err)
	}
	return id, nil
}

func (r *fileRepository) get(ctx context.Context, viewerID, id uuid.UUID) (*fileRow, error) {
	query := `
		SELECT ` + fileColumns + `
		FROM course_files f
		JOIN ready_media m ON m.id = f.media_id
		WHERE f.id = $1 AND ` + notBlocked("$2", "f.author_id")
	row, err := scanFile(r.db.Conn(ctx).QueryRow(ctx, query, id, viewerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get course file: %w", err)
	}
	return row, nil
}

func (r *fileRepository) list(ctx context.Context, f fileFilter) ([]fileRow, error) {
	var args database.Args
	arg := args.Add

	where := []string{notBlocked(arg(f.ViewerID), "f.author_id")}
	if f.CourseID != uuid.Nil {
		where = append(where, "f.course_id = "+arg(f.CourseID))
	}
	if f.AuthorID != uuid.Nil {
		where = append(where, "f.author_id = "+arg(f.AuthorID))
	}
	for _, word := range f.Words {
		pattern := database.SearchPattern(arg(word))
		// UNION lets each branch use its own trigram index.
		where = append(where, `f.id IN (
			SELECT id FROM course_files WHERE public.search_text(title) LIKE `+pattern+`
			UNION ALL
			SELECT cf.id FROM course_files cf JOIN courses sc ON sc.id = cf.course_id
			WHERE public.search_text(sc.subject) LIKE `+pattern+`)`)
	}
	if f.UniversityID != "" {
		where = append(where, "c.university_id = "+arg(f.UniversityID))
	}
	if f.Faculty != "" {
		where = append(where, "c.faculty = "+arg(f.Faculty))
	}
	if f.Subject != "" {
		where = append(where, "c.subject = "+arg(f.Subject))
	}
	if f.Semester != 0 {
		where = append(where, "c.semester = "+arg(f.Semester))
	}
	if f.After != nil {
		where = append(where, "(f.created_at, f.id) < ("+arg(f.After.CreatedAt)+", "+arg(f.After.ID)+")")
	}

	query := `
		SELECT ` + fileColumns + `
		FROM course_files f
		JOIN ready_media m ON m.id = f.media_id
		JOIN courses c ON c.id = f.course_id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY f.created_at DESC, f.id DESC
		LIMIT ` + arg(f.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list course files: %w", err)
	}
	defer rows.Close()

	var files []fileRow
	for rows.Next() {
		file, err := scanFile(rows)
		if err != nil {
			return nil, fmt.Errorf("scan course file: %w", err)
		}
		files = append(files, *file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list course files: %w", err)
	}
	return files, nil
}

func (r *fileRepository) delete(ctx context.Context, id, authorID uuid.UUID) (uuid.UUID, bool, error) {
	const query = `DELETE FROM course_files WHERE id = $1 AND author_id = $2 RETURNING media_id`
	var mediaID uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx, query, id, authorID).Scan(&mediaID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("delete course file: %w", err)
	}
	return mediaID, true, nil
}

func (r *fileRepository) deleteByCourse(ctx context.Context, courseID uuid.UUID) ([]uuid.UUID, error) {
	const query = `DELETE FROM course_files WHERE course_id = $1 RETURNING media_id`
	rows, err := r.db.Conn(ctx).Query(ctx, query, courseID)
	if err != nil {
		return nil, fmt.Errorf("delete course files by course: %w", err)
	}
	mediaIDs, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, fmt.Errorf("delete course files by course: %w", err)
	}
	return mediaIDs, nil
}

func (r *fileRepository) idsByAuthor(ctx context.Context, authorID uuid.UUID) ([]uuid.UUID, error) {
	const query = `SELECT id FROM course_files WHERE author_id = $1`
	rows, err := r.db.Conn(ctx).Query(ctx, query, authorID)
	if err != nil {
		return nil, fmt.Errorf("list course files by author: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, fmt.Errorf("list course files by author: %w", err)
	}
	return ids, nil
}
