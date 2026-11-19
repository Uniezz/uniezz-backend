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

type connectionPreferencesRow struct {
	BirthDate *time.Time
	OpenTo    []string
	Intent    *string
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
        WHERE u.id = $1 AND u.deleted_at IS NULL`

	var bannerID *uuid.UUID
	var bannerKind, bannerKey *string

	p, err := scanProfile(
		r.db.Conn(ctx).QueryRow(ctx, query, userID),
		&bannerID, &bannerKind, &bannerKey,
	)
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

func (r *profileRepository) getConnectionPreferences(ctx context.Context, userID uuid.UUID) (*connectionPreferencesRow, error) {
	const query = `
		SELECT birth_date, open_to, intent
		FROM connection_preferences
		WHERE user_id = $1
	`
	var preferences connectionPreferencesRow
	err := r.db.Conn(ctx).QueryRow(ctx, query, userID).Scan(
		&preferences.BirthDate,
		&preferences.OpenTo,
		&preferences.Intent,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		preferences.OpenTo = []string{}
		return &preferences, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get connection preferences: %w", err)
	}
	if preferences.OpenTo == nil {
		preferences.OpenTo = []string{}
	}
	return &preferences, nil
}

func (r *profileRepository) getMany(ctx context.Context, userIDs []uuid.UUID) ([]profileRow, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, "SELECT "+profileColumns+profileFrom+" WHERE u.id = ANY($1) AND u.deleted_at IS NULL", userIDs)
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

	where = append(where, "u.deleted_at IS NULL")

	query := "SELECT " + profileColumns + ", " + rank + " AS search_rank" + profileFrom
	query += " WHERE " + strings.Join(where, " AND ")
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

func (r *profileRepository) isComplete(ctx context.Context, userID uuid.UUID) (bool, error) {
	const query = `
		SELECT first_name IS NOT NULL AND last_name IS NOT NULL AND gender IS NOT NULL
			AND avatar_media_id IS NOT NULL AND banner_media_id IS NOT NULL
			AND bio IS NOT NULL AND song_url IS NOT NULL AND cardinality(interests) > 0
		FROM users
		WHERE id = $1`

	var complete bool
	if err := r.db.Conn(ctx).QueryRow(ctx, query, userID).Scan(&complete); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrNotFound
		}
		return false, fmt.Errorf("check profile complete: %w", err)
	}
	return complete, nil
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

func (r *profileRepository) markDeleting(ctx context.Context, userID uuid.UUID) (bool, error) {
	const query = `
		UPDATE users
		SET deleted_at = NOW(), email = NULL, usos_user_id = NULL
		WHERE id = $1 AND deleted_at IS NULL`

	result, err := r.db.Conn(ctx).Exec(ctx, query, userID)
	if err != nil {
		return false, fmt.Errorf("mark user deleting: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

func (r *profileRepository) deleteDeleting(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.db.Conn(ctx).Exec(ctx, `DELETE FROM users WHERE id = $1 AND deleted_at IS NOT NULL`, userID); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

func (r *profileRepository) isDeleting(ctx context.Context, userID uuid.UUID) (bool, error) {
	var deleting bool
	err := r.db.Conn(ctx).QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND deleted_at IS NOT NULL)`, userID).Scan(&deleting)
	if err != nil {
		return false, fmt.Errorf("check user deleting: %w", err)
	}
	return deleting, nil
}

func (r *profileRepository) restartStuckDeletions(ctx context.Context, olderThan time.Duration, limit int) ([]uuid.UUID, error) {
	const query = `
		UPDATE users SET deleted_at = NOW()
		WHERE id IN (
			SELECT id FROM users
			WHERE deleted_at < NOW() - $1::interval
			ORDER BY deleted_at
			LIMIT $2
			FOR UPDATE SKIP LOCKED)
		RETURNING id`

	rows, err := r.db.Conn(ctx).Query(ctx, query, olderThan, limit)
	if err != nil {
		return nil, fmt.Errorf("restart stuck deletions: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan stuck deletion: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("restart stuck deletions: %w", err)
	}
	return ids, nil
}

func (r *profileRepository) checkAvatarPhotoLimit(ctx context.Context, userID uuid.UUID) error {
	var avatarID *uuid.UUID
	const avatarQuery = `
        SELECT avatar_media_id
        FROM users
        WHERE id = $1 AND deleted_at IS NULL
        FOR UPDATE
    `
	err := r.db.Conn(ctx).QueryRow(ctx, avatarQuery, userID).Scan(&avatarID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("check avatar photo limit: %w", err)
	}

	// Replacing an existing avatar does not increase the photo count.
	if avatarID != nil {
		return nil
	}

	var photoCount int
	const countQuery = `SELECT count(*) FROM profile_photos WHERE user_id = $1`
	if err := r.db.Conn(ctx).QueryRow(ctx, countQuery, userID).Scan(&photoCount); err != nil {
		return fmt.Errorf("count profile photos for avatar: %w", err)
	}
	if photoCount >= maxProfilePhotos {
		return ErrProfilePhotoLimit
	}
	return nil
}

type profilePhotoRecord struct {
	ID       uuid.UUID
	Position int
	Ref      media.Ref
}

func (r *profileRepository) listProfilePhotos(ctx context.Context, userID uuid.UUID) ([]profilePhotoRecord, error) {
	const query = `
        SELECT pp.media_id, pp.position, rm.kind, rm.object_key
        FROM profile_photos pp
        JOIN ready_media rm ON rm.id = pp.media_id
        WHERE pp.user_id = $1
        ORDER BY pp.position
    `
	rows, err := r.db.Conn(ctx).Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list profile photos: %w", err)
	}
	defer rows.Close()

	photos := make([]profilePhotoRecord, 0)
	for rows.Next() {
		var photo profilePhotoRecord
		var kind string
		if err := rows.Scan(&photo.ID, &photo.Position, &kind, &photo.Ref.ObjectKey); err != nil {
			return nil, fmt.Errorf("scan profile photo: %w", err)
		}
		photo.Ref.Kind = media.Kind(kind)
		photos = append(photos, photo)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read profile photos: %w", err)
	}
	return photos, nil
}

func (r *profileRepository) listProfilePhotoIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	const query = `
		SELECT media_id
		FROM profile_photos
		WHERE user_id = $1
	`
	rows, err := r.db.Conn(ctx).Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list profile photo ids: %w", err)
	}
	defer rows.Close()

	mediaIDs := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan profile photo id: %w", err)
		}
		mediaIDs = append(mediaIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read profile photo ids: %w", err)
	}
	return mediaIDs, nil
}

func (r *profileRepository) replaceProfilePhotoOrder(ctx context.Context, userID uuid.UUID, mediaIDs []uuid.UUID) error {
	const deleteQuery = `DELETE FROM profile_photos WHERE user_id = $1`
	_, err := r.db.Conn(ctx).Exec(ctx, deleteQuery, userID)
	if err != nil {
		return fmt.Errorf("clear profile photo positions: %w", err)
	}

	if len(mediaIDs) == 0 {
		return nil
	}

	const insertQuery = `
        INSERT INTO profile_photos (user_id, media_id, position)
        SELECT $1, ordered.media_id, ordered.position::smallint
        FROM unnest($2::uuid[]) WITH ORDINALITY AS ordered(media_id, position)
    `
	_, err = r.db.Conn(ctx).Exec(ctx, insertQuery, userID, mediaIDs)
	if err != nil {
		return fmt.Errorf("save reordered profile photos: %w", err)
	}
	return nil
}

func (r *profileRepository) compactProfilePhotoPositions(ctx context.Context, userID uuid.UUID) error {
	const query = `
        SELECT media_id
        FROM profile_photos
        WHERE user_id = $1
        ORDER BY position
    `
	rows, err := r.db.Conn(ctx).Query(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("list remaining profile photos: %w", err)
	}

	var mediaIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("scan remaining profile photo: %w", err)
		}
		mediaIDs = append(mediaIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("read remaining profile photos: %w", err)
	}
	rows.Close()

	return r.replaceProfilePhotoOrder(ctx, userID, mediaIDs)
}

func (r *profileRepository) deleteProfilePhoto(ctx context.Context, userID, mediaID uuid.UUID) (*uuid.UUID, error) {
	var deletedID uuid.UUID
	const query = `
        DELETE FROM profile_photos
        WHERE user_id = $1 AND media_id = $2
        RETURNING media_id
    `
	err := r.db.Conn(ctx).QueryRow(ctx, query, userID, mediaID).Scan(&deletedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("remove profile photo: %w", err)
	}
	return &deletedID, nil
}

func (r *profileRepository) insertProfilePhoto(ctx context.Context, userID, mediaID uuid.UUID, position int) error {
	const query = `
        INSERT INTO profile_photos (user_id, media_id, position)
        VALUES ($1, $2, $3)
    `
	_, err := r.db.Conn(ctx).Exec(ctx, query, userID, mediaID, position)
	if err != nil {
		return fmt.Errorf("save profile photo: %w", err)
	}
	return nil
}

func (r *profileRepository) lockProfileForPhoto(ctx context.Context, userID uuid.UUID) (*uuid.UUID, error) {
	var avatarID *uuid.UUID
	const query = `
        SELECT avatar_media_id
        FROM users
        WHERE id = $1 AND deleted_at IS NULL
        FOR UPDATE
    `
	err := r.db.Conn(ctx).QueryRow(ctx, query, userID).Scan(&avatarID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock profile for photo: %w", err)
	}
	return avatarID, nil
}

func (r *profileRepository) nextProfilePhotoPosition(ctx context.Context, userID uuid.UUID) (int, error) {
	var position int
	const query = `
        SELECT candidate.position
        FROM generate_series(1, $2::int) AS candidate(position)
        WHERE NOT EXISTS (
            SELECT 1
            FROM profile_photos p
            WHERE p.user_id = $1 AND p.position = candidate.position
        )
        ORDER BY candidate.position
        LIMIT 1
    `
	err := r.db.Conn(ctx).QueryRow(ctx, query, userID, maxProfilePhotos).Scan(&position)
	if err != nil {
		return 0, fmt.Errorf("find profile photo position: %w", err)
	}
	return position, nil
}
