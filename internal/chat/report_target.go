package chat

import (
	"context"

	"github.com/google/uuid"
)

func (m Module) ReportTarget(ctx context.Context, reporterID, messageID uuid.UUID) (uuid.UUID, map[string]any, error) {
	row, err := m.messages.getVisible(ctx, reporterID, messageID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	if row.Kind != MessageText || row.DeletedAt != nil || row.SenderID == nil {
		return uuid.Nil, nil, ErrNotFound
	}

	mediaIDs, err := m.messages.mediaIDs(ctx, row.ID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	body := ""
	if row.Body != nil {
		body = *row.Body
	}
	return *row.SenderID, map[string]any{"text": body, "kind": row.Kind, "attachments": len(mediaIDs)}, nil
}
