package faq

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/rewards"
	"github.com/google/uuid"
)

// listPending gives each proposal in the language its author wrote it in.
func (m Module) listPending(ctx context.Context, cursor string, limit int) ([]Entry, *string, error) {
	return m.list(ctx, listQuery{Status: statusProposed, Cursor: cursor, Limit: limit})
}

type approvalChange struct {
	ModeratorID uuid.UUID
	EntryID     uuid.UUID
	Texts       map[string]EntryText
}

func (m Module) approve(ctx context.Context, c approvalChange) (*Entry, error) {
	texts, err := cleanAllTexts(c.Texts)
	if err != nil {
		return nil, err
	}
	var entry Entry
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		row, authorID, err := m.entries.publishProposal(ctx, c.EntryID, texts)
		if err != nil {
			return err
		}
		entry = toEntry(*row)
		if authorID == c.ModeratorID {
			return nil
		}
		_, err = m.rewards.Grant(ctx, authorID, rewards.ReasonFAQApproved, &c.EntryID, nil)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &entry, nil
}
