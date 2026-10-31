package feed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type commentRow struct {
	ID               uuid.UUID
	PostID           uuid.UUID
	ParentID         *uuid.UUID
	ReplyToCommentID *uuid.UUID
	Author           *authorRow
	Content          *string
	LikesCount       int
	RepliesCount     int
	LikedByMe        bool
	Deleted          bool
	CreatedAt        time.Time
	EditedAt         *time.Time
}

const commentSelect = `
	SELECT c.id, c.post_id, c.parent_id, c.reply_to_comment_id, c.content, c.likes_count, c.replies_count,
		c.deleted_at IS NOT NULL, c.created_at, c.edited_at,
		EXISTS (SELECT 1 FROM feed_comment_likes l WHERE l.comment_id = c.id AND l.user_id = $1),
		a.id, a.first_name, a.last_name, a.university_id, avatar.kind, avatar.object_key
	FROM feed_comments c
	LEFT JOIN public_profiles a ON a.id = c.author_id
	LEFT JOIN ready_media avatar ON avatar.id = a.avatar_media_id`

func scanComment(row pgx.Row) (*commentRow, error) {
	var c commentRow
	var authorID *uuid.UUID
	var author authorRow
	var university, avatarKind, avatarKey *string
	err := row.Scan(&c.ID, &c.PostID, &c.ParentID, &c.ReplyToCommentID, &c.Content, &c.LikesCount, &c.RepliesCount,
		&c.Deleted, &c.CreatedAt, &c.EditedAt, &c.LikedByMe,
		&authorID, &author.FirstName, &author.LastName, &university, &avatarKind, &avatarKey)
	if err != nil {
		return nil, err
	}
	if authorID != nil && university != nil {
		author.ID = *authorID
		author.UniversityID = *university
		author.Avatar = mediaRef(avatarKind, avatarKey)
		c.Author = &author
	}
	return &c, nil
}

type commentRepository struct {
	db database.DB
}

func newCommentRepository(db database.DB) *commentRepository {
	return &commentRepository{db: db}
}

func (r *commentRepository) get(ctx context.Context, viewerID, commentID uuid.UUID) (*commentRow, error) {
	c, err := scanComment(r.db.Conn(ctx).QueryRow(ctx, commentSelect+` WHERE c.id = $2`, viewerID, commentID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get comment: %w", err)
	}
	return c, nil
}

type commentCursor struct {
	CreatedAt time.Time `json:"c"`
	ID        uuid.UUID `json:"i"`
}

func (r *commentRepository) listRoots(ctx context.Context, viewerID, postID uuid.UUID, after *commentCursor, limit int) ([]commentRow, error) {
	return r.list(ctx, `c.post_id = $2 AND c.parent_id IS NULL`, viewerID, postID, after, limit)
}

func (r *commentRepository) listReplies(ctx context.Context, viewerID, rootID uuid.UUID, after *commentCursor, limit int) ([]commentRow, error) {
	return r.list(ctx, `c.parent_id = $2`, viewerID, rootID, after, limit)
}

func (r *commentRepository) list(ctx context.Context, where string, viewerID, scopeID uuid.UUID, after *commentCursor, limit int) ([]commentRow, error) {
	var afterTime *time.Time
	var afterID *uuid.UUID
	if after != nil {
		afterTime, afterID = &after.CreatedAt, &after.ID
	}

	rows, err := r.db.Conn(ctx).Query(ctx, commentSelect+`
		WHERE `+where+`
		  AND ($3::timestamptz IS NULL OR (c.created_at, c.id) > ($3, $4))
		ORDER BY c.created_at, c.id
		LIMIT $5`,
		viewerID, scopeID, afterTime, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()

	var comments []commentRow
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		comments = append(comments, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	return comments, nil
}

type commentRef struct {
	ID       uuid.UUID
	PostID   uuid.UUID
	ParentID *uuid.UUID
	ThreadID uuid.UUID
}

// ref reads the comment's immutable place in the tree without locking, so the
// caller can lock the post and the root first.
func (r *commentRepository) ref(ctx context.Context, commentID uuid.UUID) (*commentRef, error) {
	c := commentRef{ID: commentID}
	err := r.db.Conn(ctx).QueryRow(ctx,
		`SELECT post_id, parent_id, thread_id FROM feed_comments WHERE id = $1 AND deleted_at IS NULL`, commentID).
		Scan(&c.PostID, &c.ParentID, &c.ThreadID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get comment ref: %w", err)
	}
	return &c, nil
}

func (r *commentRepository) isRoot(ctx context.Context, commentID uuid.UUID) error {
	var found bool
	err := r.db.Conn(ctx).QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM feed_comments WHERE id = $1 AND parent_id IS NULL)`, commentID).Scan(&found)
	if err != nil {
		return fmt.Errorf("check root comment: %w", err)
	}
	if !found {
		return ErrNotFound
	}
	return nil
}

func (r *commentRepository) lockLive(ctx context.Context, commentID uuid.UUID) error {
	var id uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx,
		`SELECT id FROM feed_comments WHERE id = $1 AND deleted_at IS NULL FOR NO KEY UPDATE`, commentID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lock comment: %w", err)
	}
	return nil
}

type lockedComment struct {
	AuthorID     uuid.UUID
	RepliesCount int
}

func (r *commentRepository) lockForChange(ctx context.Context, commentID uuid.UUID, forDelete bool) (*lockedComment, error) {
	query := `SELECT author_id, replies_count FROM feed_comments WHERE id = $1 AND deleted_at IS NULL FOR NO KEY UPDATE`
	if forDelete {
		query = `SELECT author_id, replies_count FROM feed_comments WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`
	}
	var c lockedComment
	if err := r.db.Conn(ctx).QueryRow(ctx, query, commentID).Scan(&c.AuthorID, &c.RepliesCount); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("lock comment: %w", err)
	}
	return &c, nil
}

func (r *commentRepository) lockRoot(ctx context.Context, rootID uuid.UUID) error {
	var id uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx,
		`SELECT id FROM feed_comments WHERE id = $1 AND parent_id IS NULL FOR NO KEY UPDATE`, rootID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lock root comment: %w", err)
	}
	return nil
}

// addReply locks the root of the post and counts one more reply under it. The
// root may be a tombstone: its thread stays open while it has replies.
func (r *commentRepository) addReply(ctx context.Context, postID, rootID uuid.UUID) error {
	result, err := r.db.Conn(ctx).Exec(ctx, `
		UPDATE feed_comments SET replies_count = replies_count + 1
		WHERE id = $1 AND post_id = $2 AND parent_id IS NULL`, rootID, postID)
	if err != nil {
		return fmt.Errorf("count reply: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// removeReply returns whether the root is a tombstone left with no replies.
func (r *commentRepository) removeReply(ctx context.Context, rootID uuid.UUID) (bool, error) {
	var orphanTombstone bool
	err := r.db.Conn(ctx).QueryRow(ctx, `
		UPDATE feed_comments SET replies_count = replies_count - 1
		WHERE id = $1
		RETURNING replies_count = 0 AND deleted_at IS NOT NULL`, rootID).Scan(&orphanTombstone)
	if err != nil {
		return false, fmt.Errorf("uncount reply: %w", err)
	}
	return orphanTombstone, nil
}

func (r *commentRepository) liveInThread(ctx context.Context, commentID, threadID uuid.UUID) error {
	var found bool
	err := r.db.Conn(ctx).QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM feed_comments WHERE id = $1 AND thread_id = $2 AND deleted_at IS NULL)`,
		commentID, threadID).Scan(&found)
	if err != nil {
		return fmt.Errorf("check reply target: %w", err)
	}
	if !found {
		return ErrNotFound
	}
	return nil
}

type newComment struct {
	ID               uuid.UUID
	PostID           uuid.UUID
	AuthorID         uuid.UUID
	ParentID         *uuid.UUID
	ReplyToCommentID *uuid.UUID
	Content          string
}

func (r *commentRepository) insert(ctx context.Context, c newComment) error {
	_, err := r.db.Conn(ctx).Exec(ctx, `
		INSERT INTO feed_comments (id, post_id, author_id, parent_id, reply_to_comment_id, content)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		c.ID, c.PostID, c.AuthorID, c.ParentID, c.ReplyToCommentID, c.Content)
	if err != nil {
		return fmt.Errorf("insert comment: %w", err)
	}
	return nil
}

func (r *commentRepository) update(ctx context.Context, commentID uuid.UUID, content string) error {
	_, err := r.db.Conn(ctx).Exec(ctx, `UPDATE feed_comments SET content = $2, edited_at = NOW() WHERE id = $1`, commentID, content)
	if err != nil {
		return fmt.Errorf("update comment: %w", err)
	}
	return nil
}

func (r *commentRepository) tombstone(ctx context.Context, commentID uuid.UUID) error {
	if _, err := r.db.Conn(ctx).Exec(ctx, `DELETE FROM feed_comment_likes WHERE comment_id = $1`, commentID); err != nil {
		return fmt.Errorf("delete comment likes: %w", err)
	}
	_, err := r.db.Conn(ctx).Exec(ctx, `
		UPDATE feed_comments
		SET deleted_at = NOW(), content = NULL, author_id = NULL, likes_count = 0, edited_at = NULL
		WHERE id = $1`, commentID)
	if err != nil {
		return fmt.Errorf("tombstone comment: %w", err)
	}
	return nil
}

func (r *commentRepository) delete(ctx context.Context, commentID uuid.UUID) error {
	if _, err := r.db.Conn(ctx).Exec(ctx, `DELETE FROM feed_comments WHERE id = $1`, commentID); err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}
	return nil
}

func (r *commentRepository) idsByAuthor(ctx context.Context, authorID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Conn(ctx).Query(ctx,
		`SELECT id FROM feed_comments WHERE author_id = $1 ORDER BY parent_id IS NULL, created_at`, authorID)
	if err != nil {
		return nil, fmt.Errorf("list comments by author: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, fmt.Errorf("list comments by author: %w", err)
	}
	return ids, nil
}

func (r *commentRepository) reportTarget(ctx context.Context, commentID uuid.UUID) (uuid.UUID, map[string]any, error) {
	var authorID, postID uuid.UUID
	var content string
	err := r.db.Conn(ctx).QueryRow(ctx,
		`SELECT author_id, post_id, content FROM feed_comments WHERE id = $1 AND deleted_at IS NULL`, commentID).
		Scan(&authorID, &postID, &content)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, nil, ErrNotFound
		}
		return uuid.Nil, nil, fmt.Errorf("get reported comment: %w", err)
	}
	return authorID, map[string]any{"postId": postID, "content": content}, nil
}
