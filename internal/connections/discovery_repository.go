package connections

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

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

type discoveryFilter struct {
	ViewerID        uuid.UUID
	Mode            Mode
	Universities    []string
	Faculty         string
	Levels          []string
	Gender          string
	SharedInterests bool
	PassCooldown    time.Duration
	Limit           int
}

type discoveryRepository struct {
	db database.DB
}

func newDiscoveryRepository(db database.DB) *discoveryRepository {
	return &discoveryRepository{db: db}
}

func canMeet(arg func(any) string, mode Mode) string {
	modes := "ARRAY[" + arg(mode) + "]::text[]"
	condition := `
		t.user_id <> v.user_id
		AND v.open_to @> ` + modes + ` AND t.open_to @> ` + modes + `
		AND NOT EXISTS (SELECT 1 FROM blocked_pairs b WHERE b.user_id = v.user_id AND b.other_id = t.user_id)
		AND NOT EXISTS (
			SELECT 1 FROM connections c
			WHERE c.user_low = LEAST(v.user_id, t.user_id) AND c.user_high = GREATEST(v.user_id, t.user_id))`
	if mode == ModeDating {
		adult := "CURRENT_DATE - make_interval(years => " + arg(adultAge) + ")"
		condition += " AND v.birth_date <= " + adult + " AND t.birth_date <= " + adult
	}
	return condition
}

func (r *discoveryRepository) find(ctx context.Context, f discoveryFilter) ([]cardRow, error) {
	var args database.Args
	arg := args.Add

	where := []string{
		"v.user_id = " + arg(f.ViewerID),
		canMeet(arg, f.Mode),
		`NOT EXISTS (
			SELECT 1 FROM connection_actions a
			WHERE a.viewer_id = v.user_id AND a.target_id = t.user_id
			  AND (a.action = ` + arg(actionLike) + ` OR a.created_at > NOW() - ` + arg(f.PassCooldown) + `::interval))`,
		"NOT EXISTS (SELECT 1 FROM hidden_candidates h WHERE h.viewer_id = v.user_id AND h.target_id = t.user_id)",
	}
	if len(f.Universities) > 0 {
		where = append(where, "p.university_id = ANY("+arg(f.Universities)+")")
	}
	if f.Gender != "" {
		where = append(where, "p.gender = "+arg(f.Gender))
	}
	if f.Faculty != "" {
		where = append(where, "p.faculty = "+arg(f.Faculty))
	}
	if len(f.Levels) > 0 {
		where = append(where, "p.level = ANY("+arg(f.Levels)+")")
	}
	if f.SharedInterests {
		where = append(where, "p.interests && vp.interests")
	}

	query := `
		SELECT t.user_id, t.intent, p.bio, p.interests,
			ARRAY(SELECT i FROM unnest(p.interests) i WHERE i = ANY(vp.interests)),
			p.university_id, p.level, EXTRACT(YEAR FROM age(CURRENT_DATE, t.birth_date))::int
		FROM connection_profiles v
		JOIN anonymous_profiles vp ON vp.id = v.user_id
		JOIN connection_profiles t ON t.user_id <> v.user_id
		JOIN anonymous_profiles p ON p.id = t.user_id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY CASE WHEN p.programme = vp.programme THEN 2 ELSE 0 END
				+ CASE WHEN p.faculty = vp.faculty THEN 1 ELSE 0 END
				+ CASE WHEN p.interests && vp.interests THEN 1 ELSE 0 END DESC,
			t.user_id
		LIMIT ` + arg(f.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("find candidates: %w", err)
	}
	defer rows.Close()

	var cards []cardRow
	for rows.Next() {
		var c cardRow
		if err := rows.Scan(&c.UserID, &c.Intent, &c.Bio, &c.Interests, &c.SharedInterests,
			&c.UniversityID, &c.Level, &c.Age); err != nil {
			return nil, fmt.Errorf("scan candidate: %w", err)
		}
		cards = append(cards, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find candidates: %w", err)
	}
	return cards, nil
}

func (r *discoveryRepository) candidates(ctx context.Context, viewerID uuid.UUID, userIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	const query = `
		WITH shown AS (SELECT unnest($2::uuid[]) AS user_id),
		inserted AS (
			INSERT INTO discovery_candidates (viewer_id, user_id)
			SELECT $1, user_id FROM shown
			ON CONFLICT (viewer_id, user_id) DO NOTHING
			RETURNING user_id, id)
		SELECT user_id, id FROM inserted
		UNION ALL
		SELECT dc.user_id, dc.id FROM discovery_candidates dc JOIN shown USING (user_id)
		WHERE dc.viewer_id = $1`
	rows, err := r.db.Conn(ctx).Query(ctx, query, viewerID, userIDs)
	if err != nil {
		return nil, fmt.Errorf("save candidates: %w", err)
	}
	defer rows.Close()

	candidates := make(map[uuid.UUID]uuid.UUID, len(userIDs))
	for rows.Next() {
		var userID, candidateID uuid.UUID
		if err := rows.Scan(&userID, &candidateID); err != nil {
			return nil, fmt.Errorf("scan candidate id: %w", err)
		}
		candidates[userID] = candidateID
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("save candidates: %w", err)
	}
	return candidates, nil
}

func (r *discoveryRepository) lookup(ctx context.Context, viewerID, candidateID uuid.UUID) (uuid.UUID, error) {
	const query = `SELECT user_id FROM discovery_candidates WHERE id = $1 AND viewer_id = $2`
	var userID uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx, query, candidateID, viewerID).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("look up candidate: %w", err)
	}
	return userID, nil
}

func (r *discoveryRepository) lockPair(ctx context.Context, viewerID, targetID uuid.UUID) error {
	const query = `
		SELECT user_id FROM connection_profiles WHERE user_id IN ($1, $2)
		ORDER BY user_id FOR NO KEY UPDATE`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, viewerID, targetID); err != nil {
		return fmt.Errorf("lock connection profiles: %w", err)
	}
	return nil
}

func (r *discoveryRepository) canMeet(ctx context.Context, viewerID, targetID uuid.UUID, mode Mode) (bool, error) {
	var args database.Args
	arg := args.Add
	query := `
		SELECT EXISTS (
			SELECT 1 FROM connection_profiles v JOIN connection_profiles t ON t.user_id = ` + arg(targetID) + `
			WHERE v.user_id = ` + arg(viewerID) + ` AND ` + canMeet(arg, mode) + `)`
	var ok bool
	if err := r.db.Conn(ctx).QueryRow(ctx, query, args...).Scan(&ok); err != nil {
		return false, fmt.Errorf("check candidate pair: %w", err)
	}
	return ok, nil
}

func (r *discoveryRepository) act(ctx context.Context, viewerID, targetID uuid.UUID, mode Mode, a action) error {
	const query = `
		INSERT INTO connection_actions (viewer_id, target_id, mode, action)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (viewer_id, target_id, mode, action) DO UPDATE SET created_at = NOW()
		WHERE connection_actions.action = $5`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, viewerID, targetID, mode, a, actionPass); err != nil {
		return fmt.Errorf("save connection action: %w", err)
	}
	return nil
}

func (r *discoveryRepository) liked(ctx context.Context, viewerID, targetID uuid.UUID, mode Mode) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM connection_actions
			WHERE viewer_id = $1 AND target_id = $2 AND mode = $3 AND action = $4)`
	var found bool
	if err := r.db.Conn(ctx).QueryRow(ctx, query, viewerID, targetID, mode, actionLike).Scan(&found); err != nil {
		return false, fmt.Errorf("check like: %w", err)
	}
	return found, nil
}

func (r *discoveryRepository) hide(ctx context.Context, viewerID, targetID uuid.UUID) error {
	const query = `INSERT INTO hidden_candidates (viewer_id, target_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, viewerID, targetID); err != nil {
		return fmt.Errorf("hide candidate: %w", err)
	}
	return nil
}

func (r *discoveryRepository) snapshot(ctx context.Context, userID uuid.UUID) (*snapshotRow, error) {
	const query = `
		SELECT t.intent, p.bio, p.interests
		FROM anonymous_profiles p
		LEFT JOIN connection_profiles t ON t.user_id = p.id
		WHERE p.id = $1`
	var s snapshotRow
	err := r.db.Conn(ctx).QueryRow(ctx, query, userID).Scan(&s.Intent, &s.Bio, &s.Interests)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get candidate snapshot: %w", err)
	}
	return &s, nil
}

func (r *discoveryRepository) pruneCandidates(ctx context.Context, olderThan time.Duration) error {
	const query = `DELETE FROM discovery_candidates WHERE created_at < NOW() - $1::interval`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, olderThan); err != nil {
		return fmt.Errorf("prune candidates: %w", err)
	}
	return nil
}

func (r *discoveryRepository) prunePasses(ctx context.Context, olderThan time.Duration) error {
	const query = `DELETE FROM connection_actions WHERE action = $1 AND created_at < NOW() - $2::interval`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, actionPass, olderThan); err != nil {
		return fmt.Errorf("prune passes: %w", err)
	}
	return nil
}
