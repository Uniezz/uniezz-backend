package profile

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
)

type profileRow struct {
	ID            uuid.UUID
	UniversityID  string
	Email         *string
	FirstName     *string
	LastName      *string
	AvatarMediaID *uuid.UUID
	Avatar        *media.Ref
	Bio           *string
	Interests     []string
	Privacy       Privacy
	CreatedAt     time.Time
}

const profileSelect = `
	SELECT u.id, u.university_id, u.email, u.first_name, u.last_name, u.avatar_media_id,
		u.bio, u.interests, u.privacy, u.created_at, avatar.kind, avatar.object_key
	FROM users u
	LEFT JOIN ready_media avatar ON avatar.id = u.avatar_media_id`

func scanProfile(row pgx.Row) (*profileRow, error) {
	var p profileRow
	var avatarKind, avatarKey *string
	err := row.Scan(&p.ID, &p.UniversityID, &p.Email, &p.FirstName, &p.LastName, &p.AvatarMediaID,
		&p.Bio, &p.Interests, &p.Privacy, &p.CreatedAt, &avatarKind, &avatarKey)
	if err != nil {
		return nil, err
	}
	if avatarKind != nil && avatarKey != nil {
		p.Avatar = &media.Ref{Kind: media.Kind(*avatarKind), ObjectKey: *avatarKey}
	}
	return &p, nil
}

type profileRepository struct {
	db database.DB
}

func newProfileRepository(db database.DB) *profileRepository {
	return &profileRepository{db: db}
}

func (r *profileRepository) get(ctx context.Context, userID uuid.UUID) (*profileRow, error) {
	p, err := scanProfile(r.db.Conn(ctx).QueryRow(ctx, profileSelect+` WHERE u.id = $1`, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get profile: %w", err)
	}
	return p, nil
}

type profileCursor struct {
	CreatedAt time.Time `json:"c"`
	ID        uuid.UUID `json:"i"`
}

type profileFilter struct {
	UniversityID string
	Name         string
	After        *profileCursor
	Limit        int
}

func (r *profileRepository) list(ctx context.Context, f profileFilter) ([]profileRow, error) {
	var afterTime *time.Time
	var afterID *uuid.UUID
	if f.After != nil {
		afterTime, afterID = &f.After.CreatedAt, &f.After.ID
	}

	rows, err := r.db.Conn(ctx).Query(ctx, `
		`+profileSelect+`
		WHERE ($1 = '' OR u.university_id = $1)
		  AND ($2 = '' OR concat_ws(' ', u.first_name, u.last_name) ILIKE '%' || $2 || '%')
		  AND ($3::timestamptz IS NULL OR (u.created_at, u.id) < ($3, $4))
		ORDER BY u.created_at DESC, u.id DESC
		LIMIT $5`,
		f.UniversityID, escapeLike(f.Name), afterTime, afterID, f.Limit)
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
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
		return nil, fmt.Errorf("list profiles: %w", err)
	}
	return profiles, nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

type profileFields struct {
	FirstName *string
	LastName  *string
	SetBio    bool
	Bio       *string
	Interests []string
	Privacy   Privacy
}

func (r *profileRepository) update(ctx context.Context, userID uuid.UUID, f profileFields) error {
	const query = `
		UPDATE users
		SET first_name = COALESCE($2, first_name),
			last_name = COALESCE($3, last_name),
			bio = CASE WHEN $4 THEN $5 ELSE bio END,
			interests = COALESCE($6, interests),
			privacy = privacy || COALESCE($7::jsonb, '{}'::jsonb)
		WHERE id = $1`

	if _, err := r.db.Conn(ctx).Exec(ctx, query, userID, f.FirstName, f.LastName, f.SetBio, f.Bio, f.Interests, f.Privacy); err != nil {
		return fmt.Errorf("update profile: %w", err)
	}
	return nil
}

func (r *profileRepository) replaceAvatar(ctx context.Context, userID uuid.UUID, mediaID *uuid.UUID) (*uuid.UUID, error) {
	var previous *uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx, `SELECT avatar_media_id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&previous)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("lock avatar: %w", err)
	}

	if _, err := r.db.Conn(ctx).Exec(ctx, `UPDATE users SET avatar_media_id = $2 WHERE id = $1`, userID, mediaID); err != nil {
		return nil, fmt.Errorf("set avatar: %w", err)
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
