package profile

import (
	"context"
	"errors"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (m Module) addProfilePhoto(ctx context.Context, userID, mediaID uuid.UUID) error {
	return m.tx.WithTx(ctx, func(ctx context.Context) error {
		var avatarID *uuid.UUID
		err := m.profiles.db.Conn(ctx).QueryRow(ctx, `
            SELECT avatar_media_id
            FROM users
            WHERE id = $1 AND deleted_at IS NULL
            FOR UPDATE
        `, userID).Scan(&avatarID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock profile for photo: %w", err)
		}

		var photoCount int
		err = m.profiles.db.Conn(ctx).QueryRow(ctx,
			`SELECT count(*) FROM profile_photos WHERE user_id = $1`,
			userID,
		).Scan(&photoCount)
		if err != nil {
			return fmt.Errorf("count profile photos: %w", err)
		}

		total := photoCount
		if avatarID != nil {
			total++
		}
		if total >= 6 {
			return ErrProfilePhotoLimit
		}

		var position int
		err = m.profiles.db.Conn(ctx).QueryRow(ctx, `
            SELECT candidate.position
            FROM generate_series(1, 6) AS candidate(position)
            WHERE NOT EXISTS (
                SELECT 1
                FROM profile_photos p
                WHERE p.user_id = $1 AND p.position = candidate.position
            )
            ORDER BY candidate.position
            LIMIT 1
        `, userID).Scan(&position)
		if err != nil {
			return fmt.Errorf("find profile photo position: %w", err)
		}

		err = m.media.Claim(ctx, mediaID, userID, media.KindAvatar)
		if errors.Is(err, media.ErrNotFound) || errors.Is(err, media.ErrUploadMismatch) {
			return errors.Join(ErrInvalidProfilePhoto, err)
		}
		if err != nil {
			return fmt.Errorf("claim profile photo: %w", err)
		}

		_, err = m.profiles.db.Conn(ctx).Exec(ctx, `
            INSERT INTO profile_photos (user_id, media_id, position)
            VALUES ($1, $2, $3)
        `, userID, mediaID, position)
		if err != nil {
			return fmt.Errorf("save profile photo: %w", err)
		}
		return nil
	})
}

func (m Module) removeProfilePhoto(ctx context.Context, userID, mediaID uuid.UUID) error {
	var removed *uuid.UUID

	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		var lockedID uuid.UUID
		err := m.profiles.db.Conn(ctx).QueryRow(ctx, `
            SELECT id
            FROM users
            WHERE id = $1 AND deleted_at IS NULL
            FOR UPDATE
        `, userID).Scan(&lockedID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock profile for photo deletion: %w", err)
		}

		var deletedID uuid.UUID
		err = m.profiles.db.Conn(ctx).QueryRow(ctx, `
            DELETE FROM profile_photos
            WHERE user_id = $1 AND media_id = $2
            RETURNING media_id
        `, userID, mediaID).Scan(&deletedID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("remove profile photo: %w", err)
		}
		removed = &deletedID

		err = m.media.MarkDeleting(ctx, deletedID)
		if err != nil && !errors.Is(err, media.ErrNotFound) {
			return fmt.Errorf("mark profile photo for deletion: %w", err)
		}

		rows, err := m.profiles.db.Conn(ctx).Query(ctx, `
            SELECT media_id
            FROM profile_photos
            WHERE user_id = $1
            ORDER BY position
        `, userID)
		if err != nil {
			return fmt.Errorf("list remaining profile photos: %w", err)
		}

		var remaining []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return fmt.Errorf("scan remaining profile photo: %w", err)
			}
			remaining = append(remaining, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("read remaining profile photos: %w", err)
		}
		rows.Close()

		for i, id := range remaining {
			if _, err := m.profiles.db.Conn(ctx).Exec(ctx, `
                UPDATE profile_photos
                SET position = $3
                WHERE user_id = $1 AND media_id = $2
            `, userID, id, i+1); err != nil {
				return fmt.Errorf("compact profile photo positions: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	m.deleteMedia(ctx, removed)
	return nil
}

func (m Module) reorderProfilePhotos(ctx context.Context, userID uuid.UUID, mediaIDs []uuid.UUID) error {
	return m.tx.WithTx(ctx, func(ctx context.Context) error {
		db := m.profiles.db.Conn(ctx)

		var lockedID uuid.UUID
		err := db.QueryRow(ctx, `
            SELECT id
            FROM users
            WHERE id = $1 AND deleted_at IS NULL
            FOR UPDATE
        `, userID).Scan(&lockedID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock profile for photo reorder: %w", err)
		}

		rows, err := db.Query(ctx, `
            SELECT media_id
            FROM profile_photos
            WHERE user_id = $1
        `, userID)
		if err != nil {
			return fmt.Errorf("list profile photos for reorder: %w", err)
		}

		existing := make(map[uuid.UUID]struct{})
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return fmt.Errorf("scan profile photo for reorder: %w", err)
			}
			existing[id] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("read profile photos for reorder: %w", err)
		}
		rows.Close()

		if len(mediaIDs) != len(existing) {
			return ErrInvalidInput
		}

		seen := make(map[uuid.UUID]struct{}, len(mediaIDs))
		for _, id := range mediaIDs {
			if _, ok := existing[id]; !ok {
				return ErrInvalidInput
			}
			if _, duplicate := seen[id]; duplicate {
				return ErrInvalidInput
			}
			seen[id] = struct{}{}
		}

		if _, err := db.Exec(ctx, `DELETE FROM profile_photos WHERE user_id = $1`, userID); err != nil {
			return fmt.Errorf("clear profile photo positions: %w", err)
		}

		for i, id := range mediaIDs {
			if _, err := db.Exec(ctx, `
                INSERT INTO profile_photos (user_id, media_id, position)
                VALUES ($1, $2, $3)
            `, userID, id, i+1); err != nil {
				return fmt.Errorf("save reordered profile photo: %w", err)
			}
		}
		return nil
	})
}

func (r *profileRepository) listProfilePhotoRefs(ctx context.Context, userID uuid.UUID) ([]media.Ref, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, `
        SELECT rm.kind, rm.object_key
        FROM profile_photos pp
        JOIN ready_media rm ON rm.id = pp.media_id
        WHERE pp.user_id = $1
        ORDER BY pp.position
    `, userID)
	if err != nil {
		return nil, fmt.Errorf("list profile photo media: %w", err)
	}
	defer rows.Close()

	photos := make([]media.Ref, 0)
	for rows.Next() {
		var kind string
		var objectKey string
		if err := rows.Scan(&kind, &objectKey); err != nil {
			return nil, fmt.Errorf("scan profile photo media: %w", err)
		}
		photos = append(photos, media.Ref{Kind: media.Kind(kind), ObjectKey: objectKey})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read profile photo media: %w", err)
	}
	return photos, nil
}
