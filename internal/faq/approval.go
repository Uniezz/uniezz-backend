package faq

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/rewards"
	"github.com/google/uuid"
)

func (m Module) listPending(ctx context.Context, moderatorID uuid.UUID, cursor string, limit int) ([]Entry, *string, error) {
	if err := m.checkPublisher(ctx, moderatorID); err != nil {
		return nil, nil, err
	}
	return m.list(ctx, listQuery{Status: statusProposed, Cursor: cursor, Limit: limit})
}

func (m Module) approve(ctx context.Context, moderatorID, entryID uuid.UUID) (*Entry, error) {
	if err := m.checkPublisher(ctx, moderatorID); err != nil {
		return nil, err
	}

	var entry Entry
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		row, authorID, err := m.entries.publishProposal(ctx, entryID)
		if err != nil {
			return err
		}
		entry = toEntry(*row)
		if authorID == moderatorID {
			return nil
		}
		_, err = m.rewards.Grant(ctx, authorID, rewards.ReasonFAQApproved, &entryID, nil)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &entry, nil
}
