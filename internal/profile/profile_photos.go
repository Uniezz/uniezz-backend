package profile

import (
	"context"
	"errors"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/google/uuid"
)

const maxProfilePhotos = 6

func (m Module) addProfilePhoto(ctx context.Context, userID, mediaID uuid.UUID) error {
	p, err := m.profiles.get(ctx, userID)
	if err != nil {
		return err
	}

	existingPhotos, err := m.profiles.listProfilePhotos(ctx, userID)
	if err != nil {
		return fmt.Errorf("check existing profile photos: %w", err)
	}
	for _, photo := range existingPhotos {
		if photo.ID == mediaID {
			return nil
		}
	}

	total := len(existingPhotos)
	if p.AvatarMediaID != nil {
		total++
	}
	if total >= maxProfilePhotos {
		return ErrProfilePhotoLimit
	}

	err = m.media.CheckUpload(ctx, mediaID, userID, media.KindAvatar)
	if errors.Is(err, media.ErrNotFound) || errors.Is(err, media.ErrUploadMismatch) {
		return errors.Join(ErrInvalidProfilePhoto, err)
	}
	if err != nil {
		return fmt.Errorf("check profile photo upload: %w", err)
	}
	return m.tx.WithTx(ctx, func(ctx context.Context) error {
		avatarID, err := m.profiles.lockProfileForPhoto(ctx, userID)
		if err != nil {
			return err
		}

		existingPhotos, err := m.profiles.listProfilePhotos(ctx, userID)
		if err != nil {
			return fmt.Errorf("check existing profile photos: %w", err)
		}
		for _, photo := range existingPhotos {
			if photo.ID == mediaID {
				return nil
			}
		}

		photoCount := len(existingPhotos)

		total := photoCount
		if avatarID != nil {
			total++
		}
		if total >= maxProfilePhotos {
			return ErrProfilePhotoLimit
		}

		position, err := m.profiles.nextProfilePhotoPosition(ctx, userID)
		if err != nil {
			return err
		}

		err = m.media.MarkReady(ctx, mediaID, userID, media.KindAvatar)
		if errors.Is(err, media.ErrNotFound) {
			return errors.Join(ErrInvalidProfilePhoto, err)
		}
		if err != nil {
			return fmt.Errorf("mark profile photo ready: %w", err)
		}

		if err := m.profiles.insertProfilePhoto(ctx, userID, mediaID, position); err != nil {
			return err
		}
		return nil
	})
}

func (m Module) removeProfilePhoto(ctx context.Context, userID, mediaID uuid.UUID) error {
	var removed *uuid.UUID

	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		_, err := m.profiles.lockProfileForPhoto(ctx, userID)
		if err != nil {
			return err
		}

		removed, err = m.profiles.deleteProfilePhoto(ctx, userID, mediaID)
		if err != nil {
			return err
		}
		if removed == nil {
			return nil
		}
		deletedID := *removed

		err = m.media.MarkDeleting(ctx, deletedID)
		if err != nil && !errors.Is(err, media.ErrNotFound) {
			return fmt.Errorf("mark profile photo for deletion: %w", err)
		}

		if err := m.profiles.compactProfilePhotoPositions(ctx, userID); err != nil {
			return err
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
		_, err := m.profiles.lockProfileForPhoto(ctx, userID)
		if err != nil {
			return err
		}

		existingIDs, err := m.profiles.listProfilePhotoIDs(ctx, userID)
		if err != nil {
			return err
		}

		existing := make(map[uuid.UUID]struct{}, len(existingIDs))
		for _, id := range existingIDs {
			existing[id] = struct{}{}
		}

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

		return m.profiles.replaceProfilePhotoOrder(ctx, userID, mediaIDs)
	})
}

func (m Module) listMyProfilePhotos(ctx context.Context, userID uuid.UUID) ([]ProfilePhoto, error) {
	p, err := m.profiles.get(ctx, userID)
	if err != nil {
		return nil, err
	}

	rows, err := m.profiles.listProfilePhotos(ctx, userID)
	if err != nil {
		return nil, err
	}

	photos := make([]ProfilePhoto, 0, len(rows)+1)
	offset := 0
	if p.AvatarMediaID != nil {
		if p.Avatar == nil {
			return nil, fmt.Errorf("profile avatar %s has no ready media reference", *p.AvatarMediaID)
		}
		url, err := m.media.URLOf(ctx, *p.Avatar)
		if err != nil {
			return nil, fmt.Errorf("resolve profile avatar URL: %w", err)
		}
		photos = append(photos, ProfilePhoto{
			ID: *p.AvatarMediaID, URL: url, Position: 1, IsAvatar: true,
		})
		offset = 1
	}

	for _, row := range rows {
		url, err := m.media.URLOf(ctx, row.Ref)
		if err != nil {
			return nil, fmt.Errorf("resolve profile photo %s URL: %w", row.ID, err)
		}
		photos = append(photos, ProfilePhoto{
			ID: row.ID, URL: url, Position: row.Position + offset,
		})
	}
	return photos, nil
}
