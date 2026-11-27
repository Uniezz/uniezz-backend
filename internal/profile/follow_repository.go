package profile

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type followRow struct {
	UserID    uuid.UUID
	CreatedAt time.Time
}

type followPair struct {
	FollowerID uuid.UUID
	FollowedID uuid.UUID
}

type followCountChange struct {
	UserID    uuid.UUID
	Followers int
	Following int
}

type followFilter struct {
	ProfileID uuid.UUID
	ViewerID  uuid.UUID
	Following bool
	After     *followCursor
	Limit     int
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

func (r *followRepository) lockUsers(ctx context.Context, userIDs ...uuid.UUID) error {
	const query = `SELECT id FROM users WHERE id = ANY($1::uuid[]) ORDER BY id FOR NO KEY UPDATE`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, userIDs); err != nil {
		return fmt.Errorf("lock users: %w", err)
	}
	return nil
}

func (r *followRepository) insert(ctx context.Context, followerID, followedID uuid.UUID) (bool, error) {
	const query = `INSERT INTO profile_follows (follower_id, followed_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	result, err := r.db.Conn(ctx).Exec(ctx, query, followerID, followedID)
	if err != nil {
		return false, fmt.Errorf("insert follow: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

func (r *followRepository) delete(ctx context.Context, followerID, followedID uuid.UUID) (bool, error) {
	const query = `DELETE FROM profile_follows WHERE follower_id = $1 AND followed_id = $2`
	result, err := r.db.Conn(ctx).Exec(ctx, query, followerID, followedID)
	if err != nil {
		return false, fmt.Errorf("delete follow: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

func (r *followRepository) deleteBetween(ctx context.Context, firstID, secondID uuid.UUID) ([]followPair, error) {
	const query = `
		DELETE FROM profile_follows
		WHERE (follower_id = $1 AND followed_id = $2)
		   OR (follower_id = $2 AND followed_id = $1)
		RETURNING follower_id, followed_id`
	rows, err := r.db.Conn(ctx).Query(ctx, query, firstID, secondID)
	if err != nil {
		return nil, fmt.Errorf("delete follows between: %w", err)
	}
	return scanFollowPairs(rows)
}

func (r *followRepository) deleteAllOf(ctx context.Context, userID uuid.UUID) ([]followPair, error) {
	const query = `
		DELETE FROM profile_follows
		WHERE follower_id = $1 OR followed_id = $1
		RETURNING follower_id, followed_id`
	rows, err := r.db.Conn(ctx).Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("delete follows of user: %w", err)
	}
	return scanFollowPairs(rows)
}

func scanFollowPairs(rows pgx.Rows) ([]followPair, error) {
	defer rows.Close()

	var pairs []followPair
	for rows.Next() {
		var p followPair
		if err := rows.Scan(&p.FollowerID, &p.FollowedID); err != nil {
			return nil, fmt.Errorf("scan deleted follow: %w", err)
		}
		pairs = append(pairs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan deleted follows: %w", err)
	}
	return pairs, nil
}

func (r *followRepository) exists(ctx context.Context, followerID, followedID uuid.UUID) (bool, error) {
	const query = `SELECT EXISTS (SELECT 1 FROM profile_follows WHERE follower_id = $1 AND followed_id = $2)`
	var found bool
	if err := r.db.Conn(ctx).QueryRow(ctx, query, followerID, followedID).Scan(&found); err != nil {
		return false, fmt.Errorf("check follow: %w", err)
	}
	return found, nil
}

func (r *followRepository) increment(ctx context.Context, followerID, followedID uuid.UUID) error {
	const query = `
		INSERT INTO profile_follow_counts AS c (user_id, followers_count, following_count)
		SELECT user_id, followers, following
		FROM (VALUES ($1::uuid, 0, 1), ($2::uuid, 1, 0)) AS d (user_id, followers, following)
		ORDER BY user_id
		ON CONFLICT (user_id) DO UPDATE SET
			followers_count = c.followers_count + EXCLUDED.followers_count,
			following_count = c.following_count + EXCLUDED.following_count`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, followerID, followedID); err != nil {
		return fmt.Errorf("increment follow counts: %w", err)
	}
	return nil
}

func (r *followRepository) lockCounts(ctx context.Context, userIDs []uuid.UUID) error {
	const query = `
		SELECT user_id FROM profile_follow_counts
		WHERE user_id = ANY($1::uuid[])
		ORDER BY user_id
		FOR NO KEY UPDATE`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, userIDs); err != nil {
		return fmt.Errorf("lock follow counts: %w", err)
	}
	return nil
}

func (r *followRepository) subtract(ctx context.Context, changes []followCountChange) error {
	const query = `
		UPDATE profile_follow_counts c
		SET followers_count = c.followers_count - d.followers,
			following_count = c.following_count - d.following
		FROM unnest($1::uuid[], $2::int[], $3::int[]) AS d (user_id, followers, following)
		WHERE c.user_id = d.user_id`

	userIDs := make([]uuid.UUID, 0, len(changes))
	followers := make([]int, 0, len(changes))
	following := make([]int, 0, len(changes))
	for _, c := range changes {
		userIDs = append(userIDs, c.UserID)
		followers = append(followers, c.Followers)
		following = append(following, c.Following)
	}
	if _, err := r.db.Conn(ctx).Exec(ctx, query, userIDs, followers, following); err != nil {
		return fmt.Errorf("subtract follow counts: %w", err)
	}
	return nil
}

// Hidden students (deleted, banned, blocked by the viewer) are few, so their
// follows are found through partial indexes and the primary key instead of
// scanning every follow of the profile.
func (r *followRepository) counts(ctx context.Context, viewerID, userID uuid.UUID) (followers, following int64, err error) {
	const query = `
		WITH hidden AS (
			SELECT id FROM users WHERE deleted_at IS NOT NULL
			UNION
			SELECT id FROM users WHERE banned_at IS NOT NULL
			UNION
			SELECT other_id FROM blocked_pairs WHERE user_id = $2
		)
		SELECT
			COALESCE((SELECT followers_count FROM profile_follow_counts WHERE user_id = $1), 0)
				- (SELECT COUNT(*) FROM hidden h
				   JOIN profile_follows f ON f.follower_id = h.id AND f.followed_id = $1),
			COALESCE((SELECT following_count FROM profile_follow_counts WHERE user_id = $1), 0)
				- (SELECT COUNT(*) FROM hidden h
				   JOIN profile_follows f ON f.follower_id = $1 AND f.followed_id = h.id)`
	if err := r.db.Conn(ctx).QueryRow(ctx, query, userID, viewerID).Scan(&followers, &following); err != nil {
		return 0, 0, fmt.Errorf("count follows: %w", err)
	}
	return followers, following, nil
}

func (r *followRepository) list(ctx context.Context, f followFilter) ([]followRow, error) {
	sourceColumn, targetColumn := "f.follower_id", "f.followed_id"
	if f.Following {
		sourceColumn, targetColumn = "f.followed_id", "f.follower_id"
	}

	var args database.Args
	arg := args.Add
	where := []string{
		targetColumn + " = " + arg(f.ProfileID),
		"u.deleted_at IS NULL",
		"u.banned_at IS NULL",
		"NOT EXISTS (SELECT 1 FROM blocked_pairs b WHERE b.user_id = " + arg(f.ViewerID) + " AND b.other_id = u.id)",
	}
	if f.After != nil {
		where = append(where, "(f.created_at, "+sourceColumn+") < ("+arg(f.After.CreatedAt)+", "+arg(f.After.UserID)+")")
	}
	query := `SELECT ` + sourceColumn + `, f.created_at
		FROM profile_follows f
		JOIN users u ON u.id = ` + sourceColumn + `
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY f.created_at DESC, ` + sourceColumn + ` DESC
		LIMIT ` + arg(f.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list follows: %w", err)
	}
	defer rows.Close()

	var follows []followRow
	for rows.Next() {
		var row followRow
		if err := rows.Scan(&row.UserID, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan follow: %w", err)
		}
		follows = append(follows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list follows: %w", err)
	}
	return follows, nil
}
