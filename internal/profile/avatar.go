package profile

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/Uniezz/uniezz-backend/internal/telemetry"
	"github.com/google/uuid"
)

func (m Module) avatarURL(ctx context.Context, mediaID *uuid.UUID) *string {
	if mediaID == nil {
		return nil
	}
	url, err := m.media.URL(ctx, *mediaID)
	if err != nil {
		telemetry.ReportError(ctx, "resolve avatar URL", err, slog.String("media_id", mediaID.String()))
		return nil
	}
	return &url
}

func (m Module) claimAvatar(ctx context.Context, userID, mediaID uuid.UUID) (*uuid.UUID, error) {
	current, err := m.profiles.get(ctx, userID)
	if err != nil {
		return nil, err
	}
	if current.AvatarMediaID != nil && *current.AvatarMediaID == mediaID {
		return nil, nil
	}

	err = m.media.Claim(ctx, mediaID, userID, media.KindAvatar)
	if errors.Is(err, media.ErrNotFound) || errors.Is(err, media.ErrUploadMismatch) {
		return nil, errors.Join(ErrInvalidAvatar, err)
	}
	if err != nil {
		return nil, err
	}
	return &mediaID, nil
}

func (m Module) removeAvatar(ctx context.Context, userID uuid.UUID) error {
	var previous *uuid.UUID
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		previous, err = m.swapAvatar(ctx, userID, nil)
		return err
	})
	if err != nil {
		return err
	}

	m.deleteMedia(ctx, previous)
	return nil
}

func (m Module) swapAvatar(ctx context.Context, userID uuid.UUID, avatarID *uuid.UUID) (*uuid.UUID, error) {
	previous, err := m.profiles.replaceAvatar(ctx, userID, avatarID)
	if err != nil || previous == nil {
		return nil, err
	}
	if avatarID != nil && *previous == *avatarID {
		return nil, nil
	}

	err = m.media.MarkDeleting(ctx, *previous)
	if errors.Is(err, media.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return previous, nil
}

func (m Module) deleteMedia(ctx context.Context, mediaID *uuid.UUID) {
	if mediaID == nil {
		return
	}
	if err := m.media.Delete(ctx, *mediaID); err != nil {
		slog.WarnContext(ctx, "delete replaced avatar, left for cleanup", slog.String("media_id", mediaID.String()), slog.Any("error", err))
	}
}
