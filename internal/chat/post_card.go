package chat

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/google/uuid"
)

type CardType string

const (
	CardPost   CardType = "post"
	CardCourse CardType = "course"
	CardNotes  CardType = "notes"
)

type cardChange struct {
	ItemType CardType
	ItemID   uuid.UUID
}

type cardPayload struct {
	ItemType CardType  `json:"itemType"`
	ItemID   uuid.UUID `json:"itemId"`
}

var cardBodies = map[CardType]string{
	CardPost:   "Shared a post",
	CardCourse: "Shared a course",
	CardNotes:  "Shared notes",
}

func (c cardChange) payload() (cardPayload, error) {
	if _, ok := cardBodies[c.ItemType]; !ok || c.ItemID == uuid.Nil {
		return cardPayload{}, ErrInvalidInput
	}
	return cardPayload(c), nil
}

type resolvedCard struct {
	Title    string
	Subtitle string
	Image    *media.Ref
}

func (m Module) resolveCards(ctx context.Context, viewerID uuid.UUID, byType map[CardType][]uuid.UUID) (map[uuid.UUID]resolvedCard, error) {
	resolved := map[uuid.UUID]resolvedCard{}
	if ids := byType[CardPost]; len(ids) > 0 {
		posts, err := m.posts.ChatCards(ctx, viewerID, ids)
		if err != nil {
			return nil, err
		}
		for id, c := range posts {
			resolved[id] = resolvedCard{Title: c.Title, Subtitle: c.Subtitle, Image: c.Image}
		}
	}
	if ids := byType[CardCourse]; len(ids) > 0 {
		courses, err := m.study.CourseCards(ctx, ids)
		if err != nil {
			return nil, err
		}
		for id, c := range courses {
			resolved[id] = resolvedCard{Title: c.Title, Subtitle: c.Subtitle}
		}
	}
	if ids := byType[CardNotes]; len(ids) > 0 {
		notes, err := m.study.NotesCards(ctx, viewerID, ids)
		if err != nil {
			return nil, err
		}
		for id, c := range notes {
			resolved[id] = resolvedCard{Title: c.Title, Subtitle: c.Subtitle}
		}
	}
	return resolved, nil
}

func (m Module) checkCard(ctx context.Context, senderID uuid.UUID, in sendInput) error {
	if in.Kind != MessagePost {
		return nil
	}
	var p cardPayload
	if err := json.Unmarshal(in.Payload, &p); err != nil {
		return fmt.Errorf("decode card: %w", err)
	}
	resolved, err := m.resolveCards(ctx, senderID, map[CardType][]uuid.UUID{p.ItemType: {p.ItemID}})
	if err != nil {
		return err
	}
	if _, ok := resolved[p.ItemID]; !ok {
		return ErrNotFound
	}
	return nil
}

func (m Module) cardViews(ctx context.Context, viewerID uuid.UUID, rows []messageRow) (map[uuid.UUID]*SharedCard, error) {
	cards := map[uuid.UUID]*SharedCard{}
	byType := map[CardType][]uuid.UUID{}
	for _, row := range rows {
		if row.Kind != MessagePost || row.DeletedAt != nil {
			continue
		}
		var p cardPayload
		if err := json.Unmarshal(row.Payload, &p); err != nil {
			return nil, fmt.Errorf("decode card: %w", err)
		}
		cards[row.ID] = &SharedCard{ItemType: p.ItemType, ItemID: p.ItemID}
		byType[p.ItemType] = append(byType[p.ItemType], p.ItemID)
	}
	if len(cards) == 0 || viewerID == uuid.Nil {
		return cards, nil
	}

	resolved, err := m.resolveCards(ctx, viewerID, byType)
	if err != nil {
		return nil, err
	}
	for _, card := range cards {
		item, ok := resolved[card.ItemID]
		if !ok {
			continue
		}
		title, subtitle := item.Title, item.Subtitle
		card.Available = true
		card.Title = &title
		if subtitle != "" {
			card.Subtitle = &subtitle
		}
		card.ImageURL = m.mediaURL(ctx, item.Image)
	}
	return cards, nil
}
