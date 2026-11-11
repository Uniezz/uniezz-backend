package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type profileRow struct {
	ID            uuid.UUID
	UniversityID  string
	Email         *string
	FirstName     *string
	LastName      *string
	Gender        *Gender
	Language      Language
	AvatarMediaID *uuid.UUID
	Avatar        *media.Ref
	BannerMediaID *uuid.UUID
	Banner        *media.Ref
	Bio           *string
	SongURL       *string
	Interests     []string
	Privacy       Privacy
	CreatedAt     time.Time
	SearchRank    int
}

const profileColumns = `
	u.id, u.university_id, u.email, u.first_name, u.last_name, u.avatar_media_id,
	u.bio, u.song_url, u.interests, u.privacy, u.created_at, u.gender, u.language, avatar.kind, avatar.object_key`

const profileFrom = `
	FROM users u
	LEFT JOIN ready_media avatar ON avatar.id = u.avatar_media_id`

func scanProfile(row pgx.Row, extra ...any) (*profileRow, error) {
	var p profileRow
	var avatarKind, avatarKey *string
	dest := []any{&p.ID, &p.UniversityID, &p.Email, &p.FirstName, &p.LastName, &p.AvatarMediaID,
		&p.Bio, &p.SongURL, &p.Interests, &p.Privacy, &p.CreatedAt, &p.Gender, &p.Language, &avatarKind, &avatarKey}
	if err := row.Scan(append(dest, extra...)...); err != nil {
		return nil, err
	}
	p.Avatar = media.RefOf(avatarKind, avatarKey)
	return &p, nil
}

type profileRepository struct {
	db database.DB
}

func newProfileRepository(db database.DB) *profileRepository {
	return &profileRepository{db: db}
}

func (r *profileRepository) get(ctx context.Context, userID uuid.UUID) (*profileRow, error) {
	query := `SELECT ` + profileColumns + `, u.banner_media_id, banner.kind, banner.object_key` + profileFrom + `
		LEFT JOIN ready_media banner ON banner.id = u.banner_media_id
		WHERE u.id = $1`
	var bannerID *uuid.UUID
	var bannerKind, bannerKey *string
	p, err := scanProfile(r.db.Conn(ctx).QueryRow(ctx, query, userID), &bannerID, &bannerKind, &bannerKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get profile: %w", err)
	}
	p.BannerMediaID = bannerID
	p.Banner = media.RefOf(bannerKind, bannerKey)
	return p, nil
}

func (r *profileRepository) getMany(ctx context.Context, userIDs []uuid.UUID) ([]profileRow, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, "SELECT "+profileColumns+profileFrom+" WHERE u.id = ANY($1)", userIDs)
	if err != nil {
		return nil, fmt.Errorf("get profiles: %w", err)
	}
	defer rows.Close()

	var profiles []profileRow
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, fmt.Errorf("scan profile: %w", err)
		}
		profiles = append(profiles, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get profiles: %w", err)
	}
	return profiles, nil
}

type profileCursor struct {
	SearchRank int       `json:"r,omitempty"`
	CreatedAt  time.Time `json:"c"`
	ID         uuid.UUID `json:"i"`
}

type profileFilter struct {
	ViewerID     uuid.UUID
	UniversityID string
	Words        []string
	After        *profileCursor
	Limit        int
}

func (r *profileRepository) list(ctx context.Context, f profileFilter) ([]profileRow, error) {
	var args database.Args
	arg := args.Add

	var where []string
	if f.UniversityID != "" {
		where = append(where, "u.university_id = "+arg(f.UniversityID))
	}
	rank := "0"
	if len(f.Words) > 0 {
		var nameMatches []string
		for _, word := range f.Words {
			pattern := database.SearchPattern(arg(word))
			nameMatches = append(nameMatches, "u.name_search LIKE "+pattern)
			// UNION lets each source use its own index.
			where = append(where, `u.id IN (
				SELECT id FROM users WHERE name_search LIKE `+pattern+`
				UNION ALL
				SELECT id FROM users WHERE public.search_text(interests) LIKE `+pattern+`
				UNION ALL
				SELECT id FROM users WHERE university_id IN (`+auth.MatchingUniversities(&args, pattern)+`)
				UNION ALL
				SELECT user_id FROM student_programmes WHERE public.search_text(programme_name) LIKE `+pattern+`
				UNION ALL
				SELECT sp.user_id FROM student_programmes sp JOIN users owner ON owner.id = sp.user_id
				WHERE public.search_text(sp.faculty_name) LIKE `+pattern+`
				  AND owner.privacy->>'showFaculty' IS DISTINCT FROM 'false')`)
		}
		rank = "CASE WHEN " + strings.Join(nameMatches, " AND ") + " THEN 0 ELSE 1 END"
	}
	if f.ViewerID != uuid.Nil {
		viewer := arg(f.ViewerID)
		where = append(where, "NOT EXISTS (SELECT 1 FROM blocked_pairs b WHERE b.user_id = "+viewer+" AND b.other_id = u.id)")
	}
	if f.After != nil {
		afterRank := arg(f.After.SearchRank)
		where = append(where, "("+rank+" > "+afterRank+" OR ("+rank+" = "+afterRank+
			" AND (u.created_at, u.id) < ("+arg(f.After.CreatedAt)+", "+arg(f.After.ID)+")))")
	}

	query := "SELECT " + profileColumns + ", " + rank + " AS search_rank" + profileFrom
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY search_rank, u.created_at DESC, u.id DESC LIMIT " + arg(f.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}
	defer rows.Close()

	var profiles []profileRow
	for rows.Next() {
		var rank int
		p, err := scanProfile(rows, &rank)
		if err != nil {
			return nil, fmt.Errorf("scan profile: %w", err)
		}
		p.SearchRank = rank
		profiles = append(profiles, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}
	return profiles, nil
}

type profileFields struct {
	FirstName  *string
	LastName   *string
	Language   *Language
	SetBio     bool
	Bio        *string
	SetSongURL bool
	SongURL    *string
	Interests  []string
	Privacy    Privacy
}

func (r *profileRepository) update(ctx context.Context, userID uuid.UUID, f profileFields) error {
	const query = `
		UPDATE users
		SET first_name = COALESCE($2, first_name),
			last_name = COALESCE($3, last_name),
			bio = CASE WHEN $4 THEN $5 ELSE bio END,
			song_url = CASE WHEN $6 THEN $7 ELSE song_url END,
			interests = COALESCE($8, interests),
			privacy = privacy || COALESCE($9::jsonb, '{}'::jsonb),
			language = COALESCE($10, language)
		WHERE id = $1`

	if _, err := r.db.Conn(ctx).Exec(ctx, query, userID, f.FirstName, f.LastName, f.SetBio, f.Bio, f.SetSongURL, f.SongURL, f.Interests, f.Privacy, f.Language); err != nil {
		return fmt.Errorf("update profile: %w", err)
	}
	return nil
}

func (r *profileRepository) blocked(ctx context.Context, a, b uuid.UUID) (bool, error) {
	var found bool
	err := r.db.Conn(ctx).QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM blocked_pairs WHERE user_id = $1 AND other_id = $2)`, a, b).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("check block: %w", err)
	}
	return found, nil
}

func (r *profileRepository) setGender(ctx context.Context, userID uuid.UUID, gender Gender) error {
	result, err := r.db.Conn(ctx).Exec(ctx, `UPDATE users SET gender = $2 WHERE id = $1 AND gender IS NULL`, userID, gender)
	if err != nil {
		return fmt.Errorf("set gender: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrGenderImmutable
	}
	return nil
}

func (r *profileRepository) replaceMedia(ctx context.Context, userID uuid.UUID, slot mediaSlot, mediaID *uuid.UUID) (*uuid.UUID, error) {
	var previous *uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx, `SELECT `+slot.column+` FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&previous)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("lock %s: %w", slot.kind, err)
	}

	if _, err := r.db.Conn(ctx).Exec(ctx, `UPDATE users SET `+slot.column+` = $2 WHERE id = $1`, userID, mediaID); err != nil {
		return nil, fmt.Errorf("set %s: %w", slot.kind, err)
	}
	return previous, nil
}

func (r *profileRepository) setAvatarIfEmpty(ctx context.Context, userID, mediaID uuid.UUID) (bool, error) {
	result, err := r.db.Conn(ctx).Exec(ctx, `UPDATE users SET avatar_media_id = $2 WHERE id = $1 AND avatar_media_id IS NULL`, userID, mediaID)
	if err != nil {
		return false, fmt.Errorf("set imported avatar: %w", err)
	}
	return result.RowsAffected() == 1, nil
}
