package profile

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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
	Bio           *string
	Interests     []string
	Privacy       Privacy
	CreatedAt     time.Time
	SearchRank    int
}

const profileColumns = `
	u.id, u.university_id, u.email, u.first_name, u.last_name, u.avatar_media_id,
	u.bio, u.interests, u.privacy, u.created_at, u.gender, u.language, avatar.kind, avatar.object_key`

const profileFrom = `
	FROM users u
	LEFT JOIN ready_media avatar ON avatar.id = u.avatar_media_id`

const profileSelect = `SELECT ` + profileColumns + profileFrom

func scanProfile(row pgx.Row, extra ...any) (*profileRow, error) {
	var p profileRow
	var avatarKind, avatarKey *string
	dest := []any{&p.ID, &p.UniversityID, &p.Email, &p.FirstName, &p.LastName, &p.AvatarMediaID,
		&p.Bio, &p.Interests, &p.Privacy, &p.CreatedAt, &p.Gender, &p.Language, &avatarKind, &avatarKey}
	if err := row.Scan(append(dest, extra...)...); err != nil {
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
	SearchRank int       `json:"r,omitempty"`
	CreatedAt  time.Time `json:"c"`
	ID         uuid.UUID `json:"i"`
}

type profileFilter struct {
	UniversityID string
	Words        []string
	After        *profileCursor
	Limit        int
}

func (r *profileRepository) list(ctx context.Context, f profileFilter) ([]profileRow, error) {
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}

	var where []string
	if f.UniversityID != "" {
		where = append(where, "u.university_id = "+arg(f.UniversityID))
	}
	rank := "0"
	if len(f.Words) > 0 {
		ids, names := auth.UniversityIDsAndNames()
		idsArg, namesArg := arg(ids), arg(names)
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
				SELECT id FROM users WHERE university_id IN (
					SELECT id FROM unnest(`+idsArg+`::text[], `+namesArg+`::text[]) AS universities(id, name)
					WHERE public.search_text(id) LIKE `+pattern+` OR public.search_text(name) LIKE `+pattern+`)
				UNION ALL
				SELECT user_id FROM student_programmes WHERE public.search_text(programme_name) LIKE `+pattern+`
				UNION ALL
				SELECT sp.user_id FROM student_programmes sp JOIN users owner ON owner.id = sp.user_id
				WHERE public.search_text(sp.faculty_name) LIKE `+pattern+`
				  AND owner.privacy->>'showFaculty' IS DISTINCT FROM 'false')`)
		}
		rank = "CASE WHEN " + strings.Join(nameMatches, " AND ") + " THEN 0 ELSE 1 END"
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
	FirstName *string
	LastName  *string
	Language  *Language
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
			privacy = privacy || COALESCE($7::jsonb, '{}'::jsonb),
			language = COALESCE($8, language)
		WHERE id = $1`

	if _, err := r.db.Conn(ctx).Exec(ctx, query, userID, f.FirstName, f.LastName, f.SetBio, f.Bio, f.Interests, f.Privacy, f.Language); err != nil {
		return fmt.Errorf("update profile: %w", err)
	}
	return nil
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
