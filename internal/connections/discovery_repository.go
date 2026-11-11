package connections

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const canMeet = `
	t.user_id <> v.user_id
	AND $2 = ANY(v.open_to) AND $2 = ANY(t.open_to)
	AND ($2 <> 'dating' OR (
		v.birth_date <= CURRENT_DATE - INTERVAL '18 years'
		AND t.birth_date <= CURRENT_DATE - INTERVAL '18 years'))
	AND NOT EXISTS (SELECT 1 FROM blocked_pairs b WHERE b.user_id = v.user_id AND b.other_id = t.user_id)
	AND NOT EXISTS (
		SELECT 1 FROM connections c
		WHERE c.user_low = LEAST(v.user_id, t.user_id) AND c.user_high = GREATEST(v.user_id, t.user_id))`

type cardRow struct {
	UserID          uuid.UUID
	Intent          *string
	Bio             *string
	Interests       []string
	SharedInterests []string
	UniversityID    string
	Level           *string
	Age             *int
}

type snapshotRow struct {
	Intent    *string
	Bio       *string
	Interests []string
}

type discoveryRepository struct {
	db database.DB
}

func newDiscoveryRepository(db database.DB) *discoveryRepository {
	return &discoveryRepository{db: db}
}

func (r *discoveryRepository) find(ctx context.Context, q discoverQuery) ([]cardRow, error) {
	args := database.Args{q.ViewerID, q.Mode}
	arg := args.Add

	where := []string{"v.user_id = $1", canMeet, `
		NOT EXISTS (
			SELECT 1 FROM connection_actions a
			WHERE a.viewer_id = v.user_id AND a.target_id = t.user_id
			  AND (a.action = 'like' OR a.created_at > NOW() - INTERVAL '30 days'))`, `
		NOT EXISTS (SELECT 1 FROM hidden_candidates h WHERE h.viewer_id = v.user_id AND h.target_id = t.user_id)`}
	if len(q.Universities) > 0 {
		where = append(where, "p.university_id = ANY("+arg(q.Universities)+")")
	}
	if q.Gender != "" {
		where = append(where, "p.gender = "+arg(q.Gender))
	}
	if q.Faculty != "" {
		where = append(where, "p.faculty = "+arg(q.Faculty))
	}
	if len(q.Levels) > 0 {
		where = append(where, "p.level = ANY("+arg(q.Levels)+")")
	}
	if q.Shared {
		where = append(where, "p.interests && vp.interests")
	}

	query := `
		SELECT t.user_id, t.intent, p.bio, p.interests,
			ARRAY(SELECT i FROM unnest(p.interests) i WHERE i = ANY(vp.interests)),
			p.university_id, p.level, EXTRACT(YEAR FROM age(CURRENT_DATE, t.birth_date))::int,
			CASE WHEN p.programme = vp.programme THEN 2 ELSE 0 END
				+ CASE WHEN p.faculty = vp.faculty THEN 1 ELSE 0 END
				+ CASE WHEN p.interests && vp.interests THEN 1 ELSE 0 END AS score
		FROM connection_profiles v
		JOIN anonymous_profiles vp ON vp.id = v.user_id
		JOIN connection_profiles t ON $2 = ANY(t.open_to)
		JOIN anonymous_profiles p ON p.id = t.user_id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY score DESC, t.user_id
		LIMIT ` + arg(q.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}
	defer rows.Close()

	var cards []cardRow
	for rows.Next() {
		var c cardRow
		var score int
		if err := rows.Scan(&c.UserID, &c.Intent, &c.Bio, &c.Interests, &c.SharedInterests,
			&c.UniversityID, &c.Level, &c.Age, &score); err != nil {
			return nil, fmt.Errorf("scan discovery card: %w", err)
		}
		cards = append(cards, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}
	return cards, nil
}

func (r *discoveryRepository) candidates(ctx context.Context, viewerID uuid.UUID, userIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, `
		WITH shown AS (SELECT unnest($2::uuid[]) AS user_id),
		inserted AS (
			INSERT INTO discovery_candidates (viewer_id, user_id)
			SELECT $1, user_id FROM shown
			ON CONFLICT (viewer_id, user_id) DO NOTHING
			RETURNING user_id, id)
		SELECT user_id, id FROM inserted
		UNION ALL
		SELECT dc.user_id, dc.id FROM discovery_candidates dc JOIN shown USING (user_id)
		WHERE dc.viewer_id = $1`, viewerID, userIDs)
	if err != nil {
		return nil, fmt.Errorf("save candidates: %w", err)
	}
	defer rows.Close()

	candidates := make(map[uuid.UUID]uuid.UUID, len(userIDs))
	for rows.Next() {
		var userID, candidateID uuid.UUID
		if err := rows.Scan(&userID, &candidateID); err != nil {
			return nil, fmt.Errorf("scan candidate: %w", err)
		}
		candidates[userID] = candidateID
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("save candidates: %w", err)
	}
	return candidates, nil
}

func (r *discoveryRepository) lookup(ctx context.Context, viewerID, candidateID uuid.UUID) (uuid.UUID, error) {
	var userID uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx, `
		SELECT user_id FROM discovery_candidates WHERE id = $1 AND viewer_id = $2`, candidateID, viewerID).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("lookup candidate: %w", err)
	}
	return userID, nil
}

func (r *discoveryRepository) lockPair(ctx context.Context, viewerID, targetID uuid.UUID, mode string) (bool, error) {
	_, err := r.db.Conn(ctx).Exec(ctx, `
		SELECT user_id FROM connection_profiles WHERE user_id IN ($1, $2)
		ORDER BY user_id FOR NO KEY UPDATE`, viewerID, targetID)
	if err != nil {
		return false, fmt.Errorf("lock connection profiles: %w", err)
	}

	var eligible bool
	err = r.db.Conn(ctx).QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM connection_profiles v JOIN connection_profiles t ON t.user_id = $3
			WHERE v.user_id = $1 AND `+canMeet+`)`, viewerID, mode, targetID).Scan(&eligible)
	if err != nil {
		return false, fmt.Errorf("check pair: %w", err)
	}
	return eligible, nil
}

func (r *discoveryRepository) act(ctx context.Context, viewerID, targetID uuid.UUID, mode, action string) error {
	_, err := r.db.Conn(ctx).Exec(ctx, `
		INSERT INTO connection_actions (viewer_id, target_id, mode, action)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (viewer_id, target_id, mode, action) DO UPDATE SET created_at = NOW()
		WHERE connection_actions.action = 'pass'`, viewerID, targetID, mode, action)
	if err != nil {
		return fmt.Errorf("save connection action: %w", err)
	}
	return nil
}

func (r *discoveryRepository) liked(ctx context.Context, viewerID, targetID uuid.UUID, mode string) (bool, error) {
	var found bool
	err := r.db.Conn(ctx).QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM connection_actions
			WHERE viewer_id = $1 AND target_id = $2 AND mode = $3 AND action = 'like')`,
		viewerID, targetID, mode).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("check like: %w", err)
	}
	return found, nil
}

func (r *discoveryRepository) hide(ctx context.Context, viewerID, targetID uuid.UUID) error {
	_, err := r.db.Conn(ctx).Exec(ctx, `
		INSERT INTO hidden_candidates (viewer_id, target_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, viewerID, targetID)
	if err != nil {
		return fmt.Errorf("hide candidate: %w", err)
	}
	return nil
}

func (r *discoveryRepository) snapshot(ctx context.Context, userID uuid.UUID) (*snapshotRow, error) {
	var s snapshotRow
	err := r.db.Conn(ctx).QueryRow(ctx, `
		SELECT t.intent, p.bio, p.interests
		FROM anonymous_profiles p
		LEFT JOIN connection_profiles t ON t.user_id = p.id
		WHERE p.id = $1`, userID).Scan(&s.Intent, &s.Bio, &s.Interests)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load candidate snapshot: %w", err)
	}
	return &s, nil
}
