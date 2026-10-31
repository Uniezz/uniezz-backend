package feed

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type authorRow struct {
	ID           uuid.UUID
	FirstName    *string
	LastName     *string
	UniversityID string
	Avatar       *media.Ref
}

type postRow struct {
	ID            uuid.UUID
	Author        authorRow
	UniversityID  string
	Type          string
	Title         string
	Content       string
	EventDate     *time.Time
	Tags          []string
	LikesCount    int
	CommentsCount int
	LikedByMe     bool
	CreatedAt     time.Time
	EditedAt      *time.Time
}

const postSelect = `
	SELECT p.id, p.university_id, p.type, p.title, p.content, p.event_date, p.tags,
		p.likes_count, p.comments_count, p.created_at, p.edited_at,
		EXISTS (SELECT 1 FROM feed_post_likes l WHERE l.post_id = p.id AND l.user_id = $1),
		a.id, a.first_name, a.last_name, a.university_id, avatar.kind, avatar.object_key
	FROM feed_posts p
	JOIN public_profiles a ON a.id = p.author_id
	LEFT JOIN ready_media avatar ON avatar.id = a.avatar_media_id`

func scanPost(row pgx.Row) (*postRow, error) {
	var p postRow
	var avatarKind, avatarKey *string
	err := row.Scan(&p.ID, &p.UniversityID, &p.Type, &p.Title, &p.Content, &p.EventDate, &p.Tags,
		&p.LikesCount, &p.CommentsCount, &p.CreatedAt, &p.EditedAt, &p.LikedByMe,
		&p.Author.ID, &p.Author.FirstName, &p.Author.LastName, &p.Author.UniversityID, &avatarKind, &avatarKey)
	if err != nil {
		return nil, err
	}
	p.Author.Avatar = mediaRef(avatarKind, avatarKey)
	return &p, nil
}

func mediaRef(kind, key *string) *media.Ref {
	if kind == nil || key == nil {
		return nil
	}
	return &media.Ref{Kind: media.Kind(*kind), ObjectKey: *key}
}

type postRepository struct {
	db database.DB
}

func newPostRepository(db database.DB) *postRepository {
	return &postRepository{db: db}
}

func (r *postRepository) authorUniversity(ctx context.Context, userID uuid.UUID) (string, error) {
	var university string
	err := r.db.Conn(ctx).QueryRow(ctx, `SELECT university_id FROM public_profiles WHERE id = $1`, userID).Scan(&university)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("get author university: %w", err)
	}
	return university, nil
}

type newPost struct {
	ID           uuid.UUID
	AuthorID     uuid.UUID
	UniversityID string
	Type         string
	Title        string
	Content      string
	EventDate    *time.Time
	Tags         []string
}

func (r *postRepository) insert(ctx context.Context, p newPost) error {
	_, err := r.db.Conn(ctx).Exec(ctx, `
		INSERT INTO feed_posts (id, author_id, university_id, type, title, content, event_date, tags)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		p.ID, p.AuthorID, p.UniversityID, p.Type, p.Title, p.Content, p.EventDate, p.Tags)
	if err != nil {
		return fmt.Errorf("insert post: %w", err)
	}
	return nil
}

func (r *postRepository) attachMedia(ctx context.Context, postID uuid.UUID, position int, mediaID uuid.UUID) error {
	_, err := r.db.Conn(ctx).Exec(ctx, `INSERT INTO feed_post_media (post_id, position, media_id) VALUES ($1, $2, $3)`, postID, position, mediaID)
	if err != nil {
		return fmt.Errorf("attach post media: %w", err)
	}
	return nil
}

func (r *postRepository) get(ctx context.Context, viewerID, postID uuid.UUID) (*postRow, error) {
	p, err := scanPost(r.db.Conn(ctx).QueryRow(ctx, postSelect+` WHERE p.id = $2`, viewerID, postID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get post: %w", err)
	}
	return p, nil
}

// pageCursor is the keyset position of the last item on a page, for posts
// and comments alike.
type pageCursor struct {
	CreatedAt time.Time `json:"c"`
	ID        uuid.UUID `json:"i"`
}

type postFilter struct {
	ViewerID     uuid.UUID
	UniversityID string
	Type         string
	From         *time.Time
	To           *time.Time
	After        *pageCursor
	Limit        int
}

func (r *postRepository) list(ctx context.Context, f postFilter) ([]postRow, error) {
	args := []any{f.ViewerID}
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}

	var where []string
	switch f.UniversityID {
	case "":
	case allUniversities:
		where = append(where, "p.university_id = "+arg(allUniversities))
	default:
		where = append(where, "p.university_id IN ("+arg(f.UniversityID)+", "+arg(allUniversities)+")")
	}
	if f.Type != "" {
		where = append(where, "p.type = "+arg(f.Type))
	}
	if f.From != nil {
		where = append(where, "p.created_at >= "+arg(*f.From))
	}
	if f.To != nil {
		where = append(where, "p.created_at < "+arg(*f.To))
	}
	if f.After != nil {
		where = append(where, "(p.created_at, p.id) < ("+arg(f.After.CreatedAt)+", "+arg(f.After.ID)+")")
	}

	query := postSelect
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY p.created_at DESC, p.id DESC LIMIT " + arg(f.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list posts: %w", err)
	}
	defer rows.Close()

	var posts []postRow
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, fmt.Errorf("scan post: %w", err)
		}
		posts = append(posts, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list posts: %w", err)
	}
	return posts, nil
}

func (r *postRepository) mediaOf(ctx context.Context, postIDs []uuid.UUID) (map[uuid.UUID][]media.Ref, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, `
		SELECT pm.post_id, m.kind, m.object_key
		FROM feed_post_media pm
		JOIN ready_media m ON m.id = pm.media_id
		WHERE pm.post_id = ANY($1)
		ORDER BY pm.post_id, pm.position`, postIDs)
	if err != nil {
		return nil, fmt.Errorf("list post media: %w", err)
	}
	defer rows.Close()

	refs := map[uuid.UUID][]media.Ref{}
	for rows.Next() {
		var postID uuid.UUID
		var ref media.Ref
		if err := rows.Scan(&postID, &ref.Kind, &ref.ObjectKey); err != nil {
			return nil, fmt.Errorf("scan post media: %w", err)
		}
		refs[postID] = append(refs[postID], ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list post media: %w", err)
	}
	return refs, nil
}

type lockedPost struct {
	AuthorID uuid.UUID
	Type     string
}

// lock takes the post row lock that serializes every change to the post and
// its comment tree. forDelete takes the stronger lock a DELETE needs anyway.
func (r *postRepository) lock(ctx context.Context, postID uuid.UUID, forDelete bool) (*lockedPost, error) {
	query := `SELECT author_id, type FROM feed_posts WHERE id = $1 FOR NO KEY UPDATE`
	if forDelete {
		query = `SELECT author_id, type FROM feed_posts WHERE id = $1 FOR UPDATE`
	}
	var p lockedPost
	if err := r.db.Conn(ctx).QueryRow(ctx, query, postID).Scan(&p.AuthorID, &p.Type); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("lock post: %w", err)
	}
	return &p, nil
}

type postChange struct {
	Title     *string
	Content   *string
	EventDate *time.Time
	Tags      []string
}

func (c postChange) empty() bool {
	return c.Title == nil && c.Content == nil && c.EventDate == nil && c.Tags == nil
}

func (r *postRepository) update(ctx context.Context, postID uuid.UUID, c postChange) error {
	_, err := r.db.Conn(ctx).Exec(ctx, `
		UPDATE feed_posts
		SET title = COALESCE($2, title),
			content = COALESCE($3, content),
			event_date = COALESCE($4, event_date),
			tags = COALESCE($5, tags),
			edited_at = NOW()
		WHERE id = $1`,
		postID, c.Title, c.Content, c.EventDate, c.Tags)
	if err != nil {
		return fmt.Errorf("update post: %w", err)
	}
	return nil
}

func (r *postRepository) mediaIDs(ctx context.Context, postID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, `SELECT media_id FROM feed_post_media WHERE post_id = $1 ORDER BY position`, postID)
	if err != nil {
		return nil, fmt.Errorf("list post media ids: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, fmt.Errorf("list post media ids: %w", err)
	}
	return ids, nil
}

func (r *postRepository) delete(ctx context.Context, postID uuid.UUID) error {
	if _, err := r.db.Conn(ctx).Exec(ctx, `DELETE FROM feed_posts WHERE id = $1`, postID); err != nil {
		return fmt.Errorf("delete post: %w", err)
	}
	return nil
}

func (r *postRepository) idsByAuthor(ctx context.Context, authorID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, `SELECT id FROM feed_posts WHERE author_id = $1 ORDER BY id`, authorID)
	if err != nil {
		return nil, fmt.Errorf("list posts by author: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, fmt.Errorf("list posts by author: %w", err)
	}
	return ids, nil
}

func (r *postRepository) countComments(ctx context.Context, postID uuid.UUID, delta int) error {
	result, err := r.db.Conn(ctx).Exec(ctx, `UPDATE feed_posts SET comments_count = comments_count + $2 WHERE id = $1`, postID, delta)
	if err != nil {
		return fmt.Errorf("count comments: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *postRepository) exists(ctx context.Context, postID uuid.UUID) error {
	var found bool
	if err := r.db.Conn(ctx).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM feed_posts WHERE id = $1)`, postID).Scan(&found); err != nil {
		return fmt.Errorf("check post: %w", err)
	}
	if !found {
		return ErrNotFound
	}
	return nil
}

func (r *postRepository) reportTarget(ctx context.Context, postID uuid.UUID) (uuid.UUID, map[string]any, error) {
	var authorID uuid.UUID
	var title, content string
	err := r.db.Conn(ctx).QueryRow(ctx, `SELECT author_id, title, content FROM feed_posts WHERE id = $1`, postID).Scan(&authorID, &title, &content)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, nil, ErrNotFound
		}
		return uuid.Nil, nil, fmt.Errorf("get reported post: %w", err)
	}
	return authorID, map[string]any{"title": title, "content": content}, nil
}
