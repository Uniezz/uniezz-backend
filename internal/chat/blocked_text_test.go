package chat

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func (e *testEnv) countRows(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

func TestBlockedText(t *testing.T) {
	e := newTestEnv(t)

	t.Run("a blocked message is rejected, not stored and not pushed", func(t *testing.T) {
		a, b := e.friends(t)
		c := e.direct(t, a, b)
		pushed := len(e.push.notices())

		rec := e.rawSend(a, c.ID, uuid.New(), map[string]any{"body": "ty k u r w o"})
		expectError(t, rec, http.StatusBadRequest, "content_not_allowed")

		if n := e.countRows(t, "SELECT count(*) FROM chat_messages WHERE conversation_id = $1 AND sender_id = $2", c.ID, a.id); n != 0 {
			t.Errorf("stored %d messages, want 0", n)
		}
		if got := len(e.push.notices()); got != pushed {
			t.Errorf("push notices = %d, want %d", got, pushed)
		}
	})

	t.Run("a message request with a blocked first message opens no conversation", func(t *testing.T) {
		sender, recipient := e.user(t), e.user(t)

		rec := e.api.Post("/chat/requests", sender.bearer, keyHeader(uuid.New()), map[string]any{"userId": recipient.id, "body": "Hej, sh1t"})
		expectError(t, rec, http.StatusBadRequest, "content_not_allowed")

		if n := e.countRows(t, "SELECT count(*) FROM chat_members WHERE user_id = $1", sender.id); n != 0 {
			t.Errorf("sender is in %d conversations, want 0", n)
		}
	})

	t.Run("a blocked group name is rejected on create and on rename", func(t *testing.T) {
		owner, friend := e.friends(t)

		rec := e.api.Post("/chat/conversations/groups", owner.bearer, map[string]any{"name": "Kurwy z roku 2", "memberIds": []uuid.UUID{friend.id}})
		expectError(t, rec, http.StatusBadRequest, "content_not_allowed")
		if n := e.countRows(t, "SELECT count(*) FROM chat_members WHERE user_id = $1", owner.id); n != 0 {
			t.Errorf("owner is in %d conversations, want 0", n)
		}

		g := e.group(t, owner, "Informatyka rok 2", friend)
		rec = e.api.Patch("/chat/conversations/"+g.ID.String(), owner.bearer, map[string]any{"name": "Хуйня рік 2"})
		expectError(t, rec, http.StatusBadRequest, "content_not_allowed")

		var name string
		if err := e.pool.QueryRow(context.Background(), "SELECT name FROM chat_conversations WHERE id = $1", g.ID).Scan(&name); err != nil {
			t.Fatalf("read group: %v", err)
		}
		if name != "Informatyka rok 2" {
			t.Errorf("group name = %q, want %q", name, "Informatyka rok 2")
		}
	})

	t.Run("ordinary messages in several languages are sent", func(t *testing.T) {
		a, b := e.friends(t)
		c := e.direct(t, a, b)
		for _, body := range []string{
			"Cześć, idziesz jutro na wykład?",
			"Привіт, підеш завтра на пару?",
			"¿Vamos juntos a la biblioteca?",
		} {
			e.send(t, a, c.ID, body)
		}
	})
}
