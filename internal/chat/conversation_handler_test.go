package chat

import (
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestEveryOperationRequiresASession(t *testing.T) {
	e := newTestEnv(t)
	id := uuid.New().String()
	key := keyHeader(uuid.New())

	tests := []struct {
		name string
		call func() int
	}{
		{"open direct", func() int { return e.api.Post("/chat/conversations/direct", map[string]any{"userId": id}).Code }},
		{"create group", func() int {
			return e.api.Post("/chat/conversations/groups", map[string]any{"name": "x", "memberIds": []string{id}}).Code
		}},
		{"list conversations", func() int { return e.api.Get("/chat/conversations").Code }},
		{"get conversation", func() int { return e.api.Get("/chat/conversations/" + id).Code }},
		{"rename conversation", func() int { return e.api.Patch("/chat/conversations/"+id, map[string]any{"name": "x"}).Code }},
		{"list members", func() int { return e.api.Get("/chat/conversations/" + id + "/members").Code }},
		{"add members", func() int {
			return e.api.Post("/chat/conversations/"+id+"/members", map[string]any{"userIds": []string{id}}).Code
		}},
		{"remove member", func() int { return e.api.Delete("/chat/conversations/" + id + "/members/" + id).Code }},
		{"leave conversation", func() int { return e.api.Delete("/chat/conversations/" + id + "/members/me").Code }},
		{"list messages", func() int { return e.api.Get("/chat/conversations/" + id + "/messages").Code }},
		{"send message", func() int {
			return e.api.Post("/chat/conversations/"+id+"/messages", key, map[string]any{"body": "x"}).Code
		}},
		{"get message", func() int { return e.api.Get("/chat/messages/" + id).Code }},
		{"delete message", func() int { return e.api.Delete("/chat/messages/" + id).Code }},
		{"mark read", func() int { return e.api.Put("/chat/conversations/"+id+"/read", map[string]any{"seq": 0}).Code }},
		{"unread totals", func() int { return e.api.Get("/chat/unread").Code }},
		{"open stream", func() int { return e.api.Get("/chat/stream").Code }},
		{"list requests", func() int { return e.api.Get("/chat/requests").Code }},
		{"accept request", func() int { return e.api.Put("/chat/requests/"+id+"/acceptance", map[string]any{}).Code }},
		{"decline request", func() int { return e.api.Delete("/chat/requests/" + id).Code }},
		{"get settings", func() int { return e.api.Get("/chat/settings/me").Code }},
		{"update settings", func() int { return e.api.Patch("/chat/settings/me", map[string]any{"whoCanMessage": "matches"}).Code }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.call(); got != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", got, http.StatusUnauthorized)
			}
		})
	}
}

func TestDirectConversation(t *testing.T) {
	e := newTestEnv(t)

	t.Run("both students get the same conversation in either order", func(t *testing.T) {
		a, b := e.friends(t)

		first := decode[Conversation](t, e.api.Post("/chat/conversations/direct", a.bearer, map[string]any{"userId": b.id}), http.StatusCreated)
		second := decode[Conversation](t, e.api.Post("/chat/conversations/direct", b.bearer, map[string]any{"userId": a.id}), http.StatusOK)
		if first.ID != second.ID {
			t.Fatalf("conversations differ: %s and %s", first.ID, second.ID)
		}
		if first.Kind != KindDirect || first.Peer == nil || first.Peer.ID == nil || *first.Peer.ID != b.id {
			t.Errorf("first = %+v, want a direct conversation with peer b", first)
		}
		if second.Peer == nil || second.Peer.ID == nil || *second.Peer.ID != a.id {
			t.Errorf("second peer = %+v, want a", second.Peer)
		}
		if first.MemberCount != nil || first.MyRole != nil {
			t.Errorf("direct conversation must not carry group fields: %+v", first)
		}
	})

	t.Run("concurrent opens create one conversation", func(t *testing.T) {
		a, b := e.friends(t)

		var wg sync.WaitGroup
		ids := make(chan uuid.UUID, 8)
		for i := range 8 {
			wg.Go(func() {
				opener, other := a, b
				if i%2 == 1 {
					opener, other = b, a
				}
				rec := e.api.Post("/chat/conversations/direct", opener.bearer, map[string]any{"userId": other.id})
				if rec.Code == http.StatusCreated || rec.Code == http.StatusOK {
					c := decode[Conversation](t, rec, rec.Code)
					ids <- c.ID
				} else {
					t.Errorf("open: %d %s", rec.Code, rec.Body.String())
				}
			})
		}
		wg.Wait()
		close(ids)

		seen := map[uuid.UUID]bool{}
		for id := range ids {
			seen[id] = true
		}
		if len(seen) != 1 {
			t.Errorf("got %d distinct conversations, want 1", len(seen))
		}
		var rows, members int
		err := e.pool.QueryRow(t.Context(), `
			SELECT COUNT(*), COALESCE(SUM((SELECT COUNT(*) FROM chat_members m WHERE m.conversation_id = c.id)), 0)
			FROM chat_conversations c WHERE c.direct_user_low = LEAST($1::uuid, $2::uuid) AND c.direct_user_high = GREATEST($1::uuid, $2::uuid)`,
			a.id, b.id).Scan(&rows, &members)
		if err != nil {
			t.Fatal(err)
		}
		if rows != 1 || members != 2 {
			t.Errorf("rows = %d members = %d, want 1 and 2", rows, members)
		}
	})

	t.Run("blocked, banned or unknown students are not found", func(t *testing.T) {
		a := e.user(t)

		friend := e.user(t)
		e.connect(t, a, friend)
		e.block(t, friend, a)
		expectError(t, e.api.Post("/chat/conversations/direct", a.bearer, map[string]any{"userId": friend.id}), http.StatusNotFound, "not_found")

		banned := e.user(t)
		e.connect(t, a, banned)
		if _, err := e.pool.Exec(t.Context(), "UPDATE users SET banned_at = NOW() WHERE id = $1", banned.id); err != nil {
			t.Fatal(err)
		}
		expectError(t, e.api.Post("/chat/conversations/direct", a.bearer, map[string]any{"userId": banned.id}), http.StatusNotFound, "not_found")
		expectError(t, e.api.Post("/chat/conversations/direct", a.bearer, map[string]any{"userId": uuid.New()}), http.StatusNotFound, "not_found")
	})

	t.Run("a student cannot chat with themselves", func(t *testing.T) {
		a := e.user(t)
		expectError(t, e.api.Post("/chat/conversations/direct", a.bearer, map[string]any{"userId": a.id}), http.StatusBadRequest, "invalid_input")
	})

	t.Run("unmatching keeps an approved conversation", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		if conversation.Origin == nil || *conversation.Origin != OriginMatch || conversation.Status == nil || *conversation.Status != StatusApproved {
			t.Fatalf("conversation = %+v, want an approved match", conversation)
		}
		e.send(t, a, conversation.ID, "hello")

		e.disconnect(a, b)
		e.send(t, b, conversation.ID, "still here")
		page := e.history(t, a, conversation.ID, "")
		if len(page.Items) != 2 {
			t.Errorf("history after unmatching = %v, want both messages", seqs(page.Items))
		}
	})
}

func TestListConversations(t *testing.T) {
	e := newTestEnv(t)

	t.Run("newest activity first, paginated, with the last message and peer", func(t *testing.T) {
		me := e.user(t)
		var conversations []Conversation
		for range 3 {
			friend := e.user(t)
			e.connect(t, me, friend)
			conversations = append(conversations, e.direct(t, me, friend))
		}
		e.send(t, me, conversations[0].ID, "newest")

		first := decode[struct {
			Items      []Conversation
			NextCursor *string
		}](t, e.api.Get("/chat/conversations?limit=2", me.bearer), http.StatusOK)
		if len(first.Items) != 2 || first.NextCursor == nil {
			t.Fatalf("first page = %d items, cursor %v", len(first.Items), first.NextCursor)
		}
		if first.Items[0].ID != conversations[0].ID || first.Items[0].LastMessage == nil || first.Items[0].LastMessage.Body == nil || *first.Items[0].LastMessage.Body != "newest" {
			t.Errorf("first item = %+v", first.Items[0])
		}

		second := decode[struct {
			Items      []Conversation
			NextCursor *string
		}](t, e.api.Get("/chat/conversations?limit=2&cursor="+*first.NextCursor, me.bearer), http.StatusOK)
		if len(second.Items) != 1 || second.NextCursor != nil {
			t.Errorf("second page = %d items, cursor %v", len(second.Items), second.NextCursor)
		}
		seen := map[uuid.UUID]bool{}
		for _, c := range append(first.Items, second.Items...) {
			seen[c.ID] = true
		}
		if len(seen) != 3 {
			t.Errorf("pages hold %d distinct conversations, want 3", len(seen))
		}
		expectError(t, e.api.Get("/chat/conversations?cursor=garbage", me.bearer), http.StatusBadRequest, "invalid_cursor")
	})

	t.Run("a non-member gets not found", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		outsider := e.user(t)
		expectError(t, e.api.Get("/chat/conversations/"+conversation.ID.String(), outsider.bearer), http.StatusNotFound, "not_found")
	})
}
