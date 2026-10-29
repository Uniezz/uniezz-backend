package profile

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var _ auth.AccountSeeder = Module{}

type ImportAvatarArgs struct {
	UserID    uuid.UUID `json:"userId"`
	SourceURL string    `json:"sourceUrl"`
}

var ImportAvatar = jobs.Define[ImportAvatarArgs]("profile.import-avatar.v1")

func (m Module) SeedAccount(ctx context.Context, tx pgx.Tx, userID uuid.UUID, seed auth.AccountSeed) (func(), error) {
	if err := m.repo.insertSeededProgrammes(ctx, tx, userID, seed.Programmes); err != nil {
		return nil, err
	}
	if seed.PhotoURL == "" {
		return nil, nil
	}

	if err := jobs.Enqueue(ctx, tx, ImportAvatar, ImportAvatarArgs{UserID: userID, SourceURL: seed.PhotoURL}); err != nil {
		return nil, err
	}
	return m.jobs.Wake, nil
}

func (m Module) importAvatar(ctx context.Context, args ImportAvatarArgs) error {
	u, err := m.repo.getUser(ctx, args.UserID)
	if errors.Is(err, ErrNotFound) || (err == nil && u.AvatarMediaID != nil) {
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

	set, err := m.repo.setAvatarIfEmpty(ctx, args.UserID, mediaID)
	if err != nil || set {
		return err
	}

	err = m.repo.inTx(ctx, func(tx pgx.Tx) error {
		return m.media.MarkDeleting(ctx, tx, mediaID)
	})
	if err != nil {
		slog.WarnContext(ctx, "discard imported avatar", slog.String("media_id", mediaID.String()), slog.Any("error", err))
		return nil
	}
	m.deleteMedia(ctx, &mediaID)
	return nil
}
