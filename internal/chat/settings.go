package chat

import (
	"context"

	"github.com/google/uuid"
)

type settingsChange struct {
	WhoCanMessage *WhoCanMessage
}

func (w WhoCanMessage) valid() bool {
	return w == WhoEveryone || w == WhoUniversity || w == WhoMatches
}

func (m Module) getSettings(ctx context.Context, userID uuid.UUID) (*Settings, error) {
	who, err := m.settings.get(ctx, userID, WhoEveryone)
	if err != nil {
		return nil, err
	}
	return &Settings{WhoCanMessage: who}, nil
}

func (m Module) updateSettings(ctx context.Context, userID uuid.UUID, c settingsChange) (*Settings, error) {
	if c.WhoCanMessage == nil {
		return m.getSettings(ctx, userID)
	}
	if !c.WhoCanMessage.valid() {
		return nil, ErrInvalidInput
	}
	if err := m.settings.setWhoCanMessage(ctx, userID, *c.WhoCanMessage); err != nil {
		return nil, err
	}
	return &Settings{WhoCanMessage: *c.WhoCanMessage}, nil
}
