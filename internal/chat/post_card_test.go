package chat

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func cardBody(itemType CardType, id uuid.UUID) map[string]any {
	return map[string]any{"card": map[string]any{"itemType": itemType, "itemId": id}}
}

func TestSendCard(t *testing.T) {
	e := newTestEnv(t)

	t.Run("a shared post resolves live for the reader, with a fixed body", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		postID := e.cards.add(CardPost, "Free tutoring")

		sent := decode[Message](t, e.rawSend(a, conversation.ID, uuid.New(), cardBody(CardPost, postID)), http.StatusCreated)
		if sent.Kind != MessagePost || sent.Body == nil || *sent.Body != "Shared a post" || sent.Card == nil || sent.Card.ItemID != postID {
			t.Fatalf("message = %+v", sent)
		}
		got := e.message(t, b, sent.ID).Card
		if !got.Available || got.Title == nil || *got.Title != "Free tutoring" || got.Subtitle == nil {
			t.Errorf("card = %+v", got)
		}
	})

	t.Run("courses and notes resolve through study", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		for _, itemType := range []CardType{CardCourse, CardNotes} {
			id := e.cards.add(itemType, "Calculus")
			sent := decode[Message](t, e.rawSend(a, conversation.ID, uuid.New(), cardBody(itemType, id)), http.StatusCreated)
			if got := e.message(t, b, sent.ID).Card; !got.Available || *got.Title != "Calculus" {
				t.Errorf("%s card = %+v", itemType, got)
			}
		}
	})

	t.Run("a deleted item or one hidden from the reader is unavailable without data", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		gone := e.cards.add(CardPost, "Old post")
		hidden := e.cards.add(CardNotes, "Notes", b.id)
		first := decode[Message](t, e.rawSend(a, conversation.ID, uuid.New(), cardBody(CardPost, gone)), http.StatusCreated)
		second := decode[Message](t, e.rawSend(a, conversation.ID, uuid.New(), cardBody(CardNotes, hidden)), http.StatusCreated)
		e.cards.remove(gone)

		for _, id := range []uuid.UUID{first.ID, second.ID} {
			got := e.message(t, b, id).Card
			if got.Available || got.Title != nil || got.Subtitle != nil || got.ImageURL != nil {
				t.Errorf("card = %+v, want unavailable and empty", got)
			}
		}
	})

	t.Run("an item the sender cannot see is not found", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		hidden := e.cards.add(CardPost, "Hidden", a.id)
		expectError(t, e.rawSend(a, conversation.ID, uuid.New(), cardBody(CardPost, hidden)), http.StatusNotFound, "not_found")
		expectError(t, e.rawSend(a, conversation.ID, uuid.New(), cardBody(CardCourse, uuid.New())), http.StatusNotFound, "not_found")
	})

	t.Run("a page without cards asks neither feed nor study", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		e.send(t, a, conversation.ID, "plain text")
		before := e.cards.callCount()

		e.history(t, b, conversation.ID, "")
		if after := e.cards.callCount(); after != before {
			t.Errorf("card lookups = %d, want none", after-before)
		}
	})
}
