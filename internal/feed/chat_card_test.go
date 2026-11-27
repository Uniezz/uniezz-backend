package feed

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestChatCards(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t.Run("a visible post gives its title and author, an empty list makes no query", func(t *testing.T) {
		author, viewer := e.user(t), e.user(t)
		post := e.createPost(t, author, nil)

		cards, err := e.module.ChatCards(ctx, viewer.id, []uuid.UUID{post.ID, uuid.New()})
		if err != nil {
			t.Fatalf("ChatCards: %v", err)
		}
		card, ok := cards[post.ID]
		if len(cards) != 1 || !ok || card.Title != post.Title || card.Subtitle == "" {
			t.Errorf("cards = %+v", cards)
		}
		empty, err := e.module.ChatCards(ctx, viewer.id, nil)
		if err != nil || len(empty) != 0 {
			t.Errorf("empty = %v, %v", empty, err)
		}
	})

	t.Run("a post by a student blocked either way is left out", func(t *testing.T) {
		author, viewer := e.user(t), e.user(t)
		post := e.createPost(t, author, nil)
		e.block(t, author, viewer)

		cards, err := e.module.ChatCards(ctx, viewer.id, []uuid.UUID{post.ID})
		if err != nil || len(cards) != 0 {
			t.Errorf("cards = %v, %v", cards, err)
		}
	})
}
