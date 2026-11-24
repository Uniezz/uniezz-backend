package profile

import (
	"context"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

type followRow struct {
	UserID    uuid.UUID
	CreatedAt time.Time
}

type followCursor struct {
	CreatedAt time.Time `json:"c"`
	UserID    uuid.UUID `json:"i"`
}

type followRepository struct {
	db database.DB
}

func newFollowRepository(db database.DB) *followRepository {
	return &followRepository{db: db}
}

func (r *followRepository) lockPair(ctx context.Context, firstID, secondID uuid.UUID) error {
	const query = `SELECT id FROM users WHERE id = ANY($1::uuid[]) ORDER BY id FOR UPDATE`
	rows, err := r.db.Conn(ctx).Query(ctx, query, []uuid.UUID{firstID, secondID})
	if err != nil {
		return fmt.Errorf("lock follow pair: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan locked follow user: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("lock follow pair: %w", err)
	}
	return nil
}

func (r *followRepository) insert(ctx context.Context, followerID, followedID uuid.UUID) error {
	const query = `INSERT INTO profile_follows (follower_id, followed_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, followerID, followedID); err != nil {
		return fmt.Errorf("follow profile: %w", err)
	}
	return nil
}

func (r *followRepository) delete(ctx context.Context, followerID, followedID uuid.UUID) error {
	const query = `DELETE FROM profile_follows WHERE follower_id = $1 AND followed_id = $2`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, followerID, followedID); err != nil {
		return fmt.Errorf("unfollow profile: %w", err)
	}
	return nil
}

func (r *followRepository) exists(ctx context.Context, followerID, followedID uuid.UUID) (bool, error) {
	const query = `SELECT EXISTS (SELECT 1 FROM profile_follows WHERE follower_id = $1 AND followed_id = $2)`
	var found bool
	if err := r.db.Conn(ctx).QueryRow(ctx, query, followerID, followedID).Scan(&found); err != nil {
		return false, fmt.Errorf("check profile follow: %w", err)
	}
	return found, nil
}

func (r *followRepository) counts(ctx context.Context, userID uuid.UUID) (followers, following int64, err error) {
	const query = `
		SELECT
			(SELECT COUNT(*)
			 FROM profile_follows f
			 JOIN users u ON u.id = f.follower_id
			 WHERE f.followed_id = $1 AND u.deleted_at IS NULL AND u.banned_at IS NULL),
			(SELECT COUNT(*)
			 FROM profile_follows f
			 JOIN users u ON u.id = f.followed_id
			 WHERE f.follower_id = $1 AND u.deleted_at IS NULL AND u.banned_at IS NULL)`
	if err := r.db.Conn(ctx).QueryRow(ctx, query, userID).Scan(&followers, &following); err != nil {
		return 0, 0, fmt.Errorf("count profile follows: %w", err)
	}
	return followers, following, nil
}

func (r *followRepository) list(
	ctx context.Context,
	profileID, viewerID uuid.UUID,
	following bool,
	after *followCursor,
	limit int,
) ([]followRow, error) {
	sourceColumn, targetColumn := "f.follower_id", "f.followed_id"
	if following {
		sourceColumn, targetColumn = "f.followed_id", "f.follower_id"
	}

	args := database.Args{profileID, viewerID}
	arg := args.Add
	where := targetColumn + ` = $1
		AND u.deleted_at IS NULL
		AND u.banned_at IS NULL
		AND NOT EXISTS (
			SELECT 1 FROM blocked_pairs b
			WHERE b.user_id = $2 AND b.other_id = u.id
		)`
	if after != nil {
		where += " AND (f.created_at, " + sourceColumn + ") < (" +
			arg(after.CreatedAt) + ", " + arg(after.UserID) + ")"
	}
	query := `SELECT ` + sourceColumn + `, f.created_at
		FROM profile_follows f
		JOIN users u ON u.id = ` + sourceColumn + `
		WHERE ` + where + `
		ORDER BY f.created_at DESC, ` + sourceColumn + ` DESC
		LIMIT ` + arg(limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list profile follows: %w", err)
	}
	defer rows.Close()

	var follows []followRow
	for rows.Next() {
		var row followRow
		if err := rows.Scan(&row.UserID, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan profile follow: %w", err)
		}
		follows = append(follows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list profile follows: %w", err)
	}
	return follows, nil
}

func (r *followRepository) removeBetween(ctx context.Context, firstID, secondID uuid.UUID) error {
	const query = `
		DELETE FROM profile_follows
		WHERE (follower_id = $1 AND followed_id = $2)
		   OR (follower_id = $2 AND followed_id = $1)`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, firstID, secondID); err != nil {
		return fmt.Errorf("remove follows between blocked users: %w", err)
	}
	return nil
}
