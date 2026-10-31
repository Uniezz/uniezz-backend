package feed

import (
	"context"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
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

func (r *likeRepository) add(ctx context.Context, t likeTarget, targetID, userID uuid.UUID) (bool, error) {
	result, err := r.db.Conn(ctx).Exec(ctx,
		`INSERT INTO `+t.likes+` (`+t.column+`, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, targetID, userID)
	if err != nil {
		return false, fmt.Errorf("add like: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

func (r *likeRepository) remove(ctx context.Context, t likeTarget, targetID, userID uuid.UUID) (bool, error) {
	result, err := r.db.Conn(ctx).Exec(ctx,
		`DELETE FROM `+t.likes+` WHERE `+t.column+` = $1 AND user_id = $2`, targetID, userID)
	if err != nil {
		return false, fmt.Errorf("remove like: %w", err)
	}
	return result.RowsAffected() == 1, nil
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
