package profile

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ImportAvatarArgs struct {
	UserID    uuid.UUID `json:"userId"`
	SourceURL string    `json:"sourceUrl"`
}

var ImportAvatar = jobs.Define[ImportAvatarArgs]("profile.import-avatar.v1")

func (m Module) importAvatar(ctx context.Context, args ImportAvatarArgs) error {
	p, err := m.profiles.get(ctx, args.UserID)
	if errors.Is(err, ErrNotFound) || (err == nil && p.AvatarMediaID != nil) {
		return nil
	}
	if err != nil {
		return err
	}

	mediaID, err := m.media.Import(ctx, args.UserID, media.KindAvatar, args.SourceURL)
	if errors.Is(err, media.ErrInvalidUpload) {
		return jobs.Permanent(err)
	}
	if err != nil {
		return err
	}

	set, err := m.profiles.setAvatarIfEmpty(ctx, args.UserID, mediaID)
	if err != nil || set {
		return err
	}

	err = m.profiles.inTx(ctx, func(tx pgx.Tx) error {
		return m.media.MarkDeleting(ctx, tx, mediaID)
	})
	if err != nil {
		slog.WarnContext(ctx, "discard imported avatar", slog.String("media_id", mediaID.String()), slog.Any("error", err))
		return nil
	}
	m.deleteMedia(ctx, &mediaID)
	return nil
}
