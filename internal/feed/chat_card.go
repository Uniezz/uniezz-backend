package feed

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/google/uuid"
)

type ChatCard struct {
	ID       uuid.UUID
	Title    string
	Subtitle string
	Image    *media.Ref
}

func (m Module) ChatCards(ctx context.Context, viewerID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]ChatCard, error) {
	cards := map[uuid.UUID]ChatCard{}
	if len(ids) == 0 {
		return cards, nil
	}
	rows, err := m.posts.chatCards(ctx, viewerID, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		cards[row.ID] = row
	}
	return cards, nil
}
