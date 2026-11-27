package study

import (
	"context"

	"github.com/google/uuid"
)

type ChatCard struct {
	ID       uuid.UUID
	Title    string
	Subtitle string
}

func (m Module) CourseCards(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]ChatCard, error) {
	cards := map[uuid.UUID]ChatCard{}
	if len(ids) == 0 {
		return cards, nil
	}
	rows, err := m.courses.chatCards(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		cards[row.ID] = row
	}
	return cards, nil
}

func (m Module) NotesCards(ctx context.Context, viewerID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]ChatCard, error) {
	cards := map[uuid.UUID]ChatCard{}
	if len(ids) == 0 {
		return cards, nil
	}
	rows, err := m.files.chatCards(ctx, viewerID, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		cards[row.ID] = row
	}
	return cards, nil
}
