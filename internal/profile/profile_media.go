package profile

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/Uniezz/uniezz-backend/internal/telemetry"
	"github.com/google/uuid"
)

type mediaSlot struct {
	column  string
	kind    media.Kind
	invalid error
	current func(*profileRow) *uuid.UUID
}

var (
	avatarSlot = mediaSlot{
		column:  "avatar_media_id",
		kind:    media.KindAvatar,
		invalid: ErrInvalidAvatar,
		current: func(p *profileRow) *uuid.UUID { return p.AvatarMediaID },
	}
	bannerSlot = mediaSlot{
		column:  "banner_media_id",
		kind:    media.KindBanner,
		invalid: ErrInvalidBanner,
		current: func(p *profileRow) *uuid.UUID { return p.BannerMediaID },
	}
)

func (m Module) mediaURL(ctx context.Context, ref *media.Ref) *string {
	if ref == nil {
		return nil
	}
	url, err := m.media.URLOf(ctx, *ref)
	if err != nil {
		telemetry.ReportError(ctx, "resolve profile media URL", err, slog.String("object_key", ref.ObjectKey))
		return nil
	}
	return &url
}

func (m Module) claimMedia(ctx context.Context, userID uuid.UUID, slot mediaSlot, mediaID *uuid.UUID) (*uuid.UUID, error) {
	if mediaID == nil {
		return nil, nil
	}
	current, err := m.profiles.get(ctx, userID)
	if err != nil {
		return nil, err
	}
	if id := slot.current(current); id != nil && *id == *mediaID {
		return nil, nil
	}

	err = m.media.Claim(ctx, *mediaID, userID, slot.kind)
	if errors.Is(err, media.ErrNotFound) || errors.Is(err, media.ErrUploadMismatch) {
		return nil, errors.Join(slot.invalid, err)
	}
	if err != nil {
		return nil, err
	}
	return mediaID, nil
}

func (m Module) removeMedia(ctx context.Context, userID uuid.UUID, slot mediaSlot) error {
	var previous *uuid.UUID
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		previous, err = m.swapMedia(ctx, userID, slot, nil)
		return err
	})
	if err != nil {
		return err
	}

	m.deleteMedia(ctx, previous)
	return nil
}

func (m Module) swapMedia(ctx context.Context, userID uuid.UUID, slot mediaSlot, mediaID *uuid.UUID) (*uuid.UUID, error) {
	previous, err := m.profiles.replaceMedia(ctx, userID, slot, mediaID)
	if err != nil || previous == nil {
		return nil, err
	}
	if mediaID != nil && *previous == *mediaID {
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
		slog.WarnContext(ctx, "delete replaced profile media, left for cleanup", slog.String("media_id", mediaID.String()), slog.Any("error", err))
	}
}
