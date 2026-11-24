package push

import (
	"context"

	"github.com/google/uuid"
)

type settingsChange struct {
	Enabled *bool
}

func (m Module) getSettings(ctx context.Context, userID uuid.UUID) (*NotificationSettings, error) {
	enabled, err := m.settings.get(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &NotificationSettings{Enabled: enabled}, nil
}

func (m Module) updateSettings(ctx context.Context, userID uuid.UUID, c settingsChange) (*NotificationSettings, error) {
	if c.Enabled == nil {
		return m.getSettings(ctx, userID)
	}
	if err := m.settings.setEnabled(ctx, userID, *c.Enabled); err != nil {
		return nil, err
	}
	return &NotificationSettings{Enabled: *c.Enabled}, nil
}
