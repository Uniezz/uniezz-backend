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

type likeTarget struct {
	likes   string
	column  string
	counted string
}

var (
	postLikes    = likeTarget{likes: "feed_post_likes", column: "post_id", counted: "feed_posts"}
	commentLikes = likeTarget{likes: "feed_comment_likes", column: "comment_id", counted: "feed_comments"}
)

type likeRepository struct {
	db database.DB
}

func newLikeRepository(db database.DB) *likeRepository {
	return &likeRepository{db: db}
}

func (r *likeRepository) add(ctx context.Context, t likeTarget, targetID, userID uuid.UUID) (*time.Time, error) {
	return r.change(ctx, "add like",
		`INSERT INTO `+t.likes+` (`+t.column+`, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING RETURNING created_at`, targetID, userID)
}

func (r *likeRepository) remove(ctx context.Context, t likeTarget, targetID, userID uuid.UUID) (*time.Time, error) {
	return r.change(ctx, "remove like",
		`DELETE FROM `+t.likes+` WHERE `+t.column+` = $1 AND user_id = $2 RETURNING created_at`, targetID, userID)
}

func (r *likeRepository) change(ctx context.Context, op, query string, targetID, userID uuid.UUID) (*time.Time, error) {
	var likedAt time.Time
	if err := r.db.Conn(ctx).QueryRow(ctx, query, targetID, userID).Scan(&likedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &likedAt, nil
}

func weekOf(timestamp string) string {
	return `date_trunc('week', ` + timestamp + ` AT TIME ZONE 'Europe/Warsaw')::date`
}

func (r *likeRepository) countWeekly(ctx context.Context, postID uuid.UUID, likedAt time.Time, delta int) error {
	query := `
		UPDATE feed_post_weekly_likes SET likes = likes + $3
		WHERE week = ` + weekOf("$2::timestamptz") + ` AND post_id = $1`
	if delta > 0 {
		query = `
			INSERT INTO feed_post_weekly_likes (week, post_id, post_created_at, likes)
			SELECT ` + weekOf("$2::timestamptz") + `, id, created_at, $3 FROM feed_posts
			WHERE id = $1 AND EXISTS (SELECT 1 FROM feed_post_media WHERE post_id = $1)
			ON CONFLICT (week, post_id) DO UPDATE SET likes = feed_post_weekly_likes.likes + EXCLUDED.likes`
	}
	if _, err := r.db.Conn(ctx).Exec(ctx, query, postID, likedAt, delta); err != nil {
		return fmt.Errorf("count weekly likes: %w", err)
	}
	return nil
}

func (r *likeRepository) pruneWeeklyBeforeThisWeek(ctx context.Context) error {
	if _, err := r.db.Conn(ctx).Exec(ctx, `DELETE FROM feed_post_weekly_likes WHERE week < `+weekOf("NOW()")); err != nil {
		return fmt.Errorf("prune weekly likes: %w", err)
	}
	return nil
}

func (r *likeRepository) count(ctx context.Context, t likeTarget, targetID uuid.UUID, delta int) (int, error) {
	var count int
	err := r.db.Conn(ctx).QueryRow(ctx,
		`UPDATE `+t.counted+` SET likes_count = likes_count + $2 WHERE id = $1 RETURNING likes_count`, targetID, delta).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count likes: %w", err)
	}
	return count, nil
}

func (r *likeRepository) current(ctx context.Context, t likeTarget, targetID uuid.UUID) (int, error) {
	var count int
	err := r.db.Conn(ctx).QueryRow(ctx, `SELECT likes_count FROM `+t.counted+` WHERE id = $1`, targetID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("read likes: %w", err)
	}
	return count, nil
}

func (r *likeRepository) postIDsLikedBy(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	return r.likedBy(ctx, postLikes, userID)
}

func (r *likeRepository) commentIDsLikedBy(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	return r.likedBy(ctx, commentLikes, userID)
}

func (r *likeRepository) likedBy(ctx context.Context, t likeTarget, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, `SELECT `+t.column+` FROM `+t.likes+` WHERE user_id = $1 ORDER BY `+t.column, userID)
	if err != nil {
		return nil, fmt.Errorf("list likes: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan like: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list likes: %w", err)
	}
	return ids, nil
}
