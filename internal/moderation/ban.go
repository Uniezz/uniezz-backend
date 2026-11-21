package moderation

import (
	"context"
	"errors"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/google/uuid"
)

func (m Module) banUser(ctx context.Context, actorID, userID uuid.UUID) error {
	err := m.staff.Ban(ctx, userID)
	if errors.Is(err, auth.ErrUserNotFound) {
		return ErrNotFound
	}
	if errors.Is(err, auth.ErrStaff) {
		return ErrTargetIsStaff
	}
	if err != nil {
		return err
	}
	if err := m.queue.resolveUser(ctx, userID, ReportReviewed); err != nil {
		return err
	}
	return m.actions.create(ctx, actorID, ActionBan, userID)
}

func (m Module) unbanUser(ctx context.Context, actorID, userID uuid.UUID) error {
	if err := m.staff.Unban(ctx, userID); err != nil {
		return err
	}
	return m.actions.create(ctx, actorID, ActionUnban, userID)
}

func (m Module) ban(ctx context.Context, userID uuid.UUID) (*Ban, error) {
	at, err := m.staff.BannedAt(ctx, userID)
	if errors.Is(err, auth.ErrUserNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &Ban{UserID: userID, BannedAt: at}, nil
}
