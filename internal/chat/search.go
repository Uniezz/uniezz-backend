package chat

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
)

type searchQuery struct {
	ViewerID       uuid.UUID
	ConversationID uuid.UUID
	Q              string
	Cursor         string
	Limit          int
}

func (q searchQuery) filter() (searchFilter, error) {
	words, err := text.SearchWords(q.Q)
	if err != nil || len(words) == 0 {
		return searchFilter{}, ErrInvalidInput
	}
	f := searchFilter{ConversationID: q.ConversationID, ViewerID: q.ViewerID, Words: words, Limit: q.Limit + 1}
	if q.Cursor != "" {
		var cursor messageCursor
		if err := httpapi.DecodeCursor(q.Cursor, &cursor); err != nil {
			return searchFilter{}, err
		}
		f.BeforeSeq = &cursor.Seq
	}
	return f, nil
}

func (m Module) searchMessages(ctx context.Context, q searchQuery) ([]Message, *string, error) {
	filter, err := q.filter()
	if err != nil {
		return nil, nil, err
	}
	a, err := m.conversations.access(ctx, q.ViewerID, q.ConversationID)
	if err != nil {
		return nil, nil, err
	}
	if err := a.viewable(); err != nil {
		return nil, nil, err
	}

	rows, err := m.messages.search(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	rows, next, err := httpapi.Page(rows, q.Limit, func(last messageRow) any {
		return messageCursor{Seq: last.Seq}
	})
	if err != nil {
		return nil, nil, err
	}

	items, err := m.messageViews(ctx, rows)
	if err != nil {
		return nil, nil, err
	}
	return items, next, nil
}
