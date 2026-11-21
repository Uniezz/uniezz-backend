package chat

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestSendMessage(t *testing.T) {
	e := newTestEnv(t)

	t.Run("a stored message carries its position, sender and client id", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		key := uuid.New()

		got := decode[Message](t, e.rawSend(a, conversation.ID, key, map[string]any{"body": " <b>Hello</b> "}), http.StatusCreated)
		if got.Seq != 1 || got.ChangeSeq != 1 || got.Kind != MessageText {
			t.Errorf("message = %+v", got)
		}
		if got.Body == nil || *got.Body != "Hello" {
			t.Errorf("body = %v, want Hello", got.Body)
		}
		if got.ClientID == nil || *got.ClientID != key {
			t.Errorf("clientId = %v, want %s", got.ClientID, key)
		}
		if got.Sender.ID == nil || *got.Sender.ID != a.id || got.Attachments == nil {
			t.Errorf("sender = %+v attachments = %v", got.Sender, got.Attachments)
		}
	})

	t.Run("20 concurrent sends get positions 1 to 20 and distinct change positions", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)

		var wg sync.WaitGroup
		for i := range 20 {
			wg.Go(func() {
				sender := a
				if i%2 == 1 {
					sender = b
				}
				e.send(t, sender, conversation.ID, "message")
			})
		}
		wg.Wait()

		rows, err := e.pool.Query(context.Background(), "SELECT seq, change_seq FROM chat_messages WHERE conversation_id = $1 ORDER BY seq", conversation.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		want := int64(1)
		changes := map[int64]bool{}
		for rows.Next() {
			var seq, change int64
			if err := rows.Scan(&seq, &change); err != nil {
				t.Fatal(err)
			}
			if seq != want {
				t.Errorf("seq = %d, want %d", seq, want)
			}
			changes[change] = true
			want++
		}
		if want != 21 || len(changes) != 20 {
			t.Errorf("got %d messages and %d distinct change_seq, want 20 and 20", want-1, len(changes))
		}

		var unread, raw int
		if err := e.pool.QueryRow(context.Background(), "SELECT unread_count FROM chat_members WHERE conversation_id = $1 AND user_id = $2", conversation.ID, a.id).Scan(&unread); err != nil {
			t.Fatal(err)
		}
		if err := e.pool.QueryRow(context.Background(), `
			SELECT COUNT(*) FROM chat_messages
			WHERE conversation_id = $1 AND sender_id <> $2
			  AND seq > (SELECT last_read_seq FROM chat_members WHERE conversation_id = $1 AND user_id = $2)`,
			conversation.ID, a.id).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if unread != raw {
			t.Errorf("unread_count = %d, raw count = %d", unread, raw)
		}
	})

	t.Run("a retry with the same key and body returns the stored message", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		key := uuid.New()
		body := map[string]any{"body": "once"}

		first := decode[Message](t, e.rawSend(a, conversation.ID, key, body), http.StatusCreated)
		rec := e.rawSend(a, conversation.ID, key, body)
		second := decode[Message](t, rec, http.StatusCreated)
		if rec.Header().Get("Idempotent-Replayed") != "true" || second.ID != first.ID {
			t.Errorf("replay = %s replayed=%q, want %s replayed", second.ID, rec.Header().Get("Idempotent-Replayed"), first.ID)
		}
		if got := e.conversation(t, a, conversation.ID); got.LastSeq != 1 || got.ChangeSeq != 1 {
			t.Errorf("replay moved the counters: lastSeq=%d changeSeq=%d", got.LastSeq, got.ChangeSeq)
		}
	})

	t.Run("ten concurrent retries of one key store one message", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		key := uuid.New()

		var wg sync.WaitGroup
		for range 10 {
			wg.Go(func() {
				rec := e.rawSend(a, conversation.ID, key, map[string]any{"body": "race"})
				if rec.Code != http.StatusCreated {
					t.Errorf("status = %d: %s", rec.Code, rec.Body.String())
				}
			})
		}
		wg.Wait()

		var rows int
		if err := e.pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM chat_messages WHERE conversation_id = $1", conversation.ID).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows != 1 {
			t.Errorf("rows = %d, want 1", rows)
		}
		if got := e.conversation(t, a, conversation.ID); got.LastSeq != 1 || got.ChangeSeq != 1 {
			t.Errorf("counters moved by replays: lastSeq=%d changeSeq=%d", got.LastSeq, got.ChangeSeq)
		}
	})

	t.Run("the same key with another body is rejected, and after a delete it is gone", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		key := uuid.New()

		first := decode[Message](t, e.rawSend(a, conversation.ID, key, map[string]any{"body": "one"}), http.StatusCreated)
		expectError(t, e.rawSend(a, conversation.ID, key, map[string]any{"body": "two"}), http.StatusUnprocessableEntity, "idempotency_key_reused")

		other := e.direct(t, a, e.befriend(t, a))
		expectError(t, e.rawSend(a, other.ID, key, map[string]any{"body": "one"}), http.StatusUnprocessableEntity, "idempotency_key_reused")

		if rec := e.api.Delete("/chat/messages/"+first.ID.String(), a.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("delete: %d", rec.Code)
		}
		expectError(t, e.rawSend(a, conversation.ID, key, map[string]any{"body": "one"}), http.StatusGone, "resource_deleted")
	})

	t.Run("invalid bodies and keys are rejected", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)

		expectError(t, e.rawSend(a, conversation.ID, uuid.New(), map[string]any{"body": "  "}), http.StatusBadRequest, "invalid_input")
		expectError(t, e.rawSend(a, conversation.ID, uuid.New(), map[string]any{}), http.StatusBadRequest, "invalid_input")
		expectError(t, e.rawSend(a, conversation.ID, uuid.New(), map[string]any{"body": strings.Repeat("a", 4001)}), http.StatusBadRequest, "invalid_input")
		decode[Message](t, e.rawSend(a, conversation.ID, uuid.New(), map[string]any{"body": strings.Repeat("ą", 4000)}), http.StatusCreated)

		rec := e.api.Post("/chat/conversations/"+conversation.ID.String()+"/messages", a.bearer, map[string]any{"body": "no key"})
		expectError(t, rec, http.StatusBadRequest, "invalid_input")
		rec = e.api.Post("/chat/conversations/"+conversation.ID.String()+"/messages", a.bearer, "Idempotency-Key: not-a-uuid-but-36-characters-long-x", map[string]any{"body": "bad key"})
		expectError(t, rec, http.StatusBadRequest, "invalid_input")
	})

	t.Run("a non-member gets not found and moves nothing", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		outsider := e.user(t)

		expectError(t, e.rawSend(outsider, conversation.ID, uuid.New(), map[string]any{"body": "hi"}), http.StatusNotFound, "not_found")
		if got := e.conversation(t, a, conversation.ID); got.LastSeq != 0 {
			t.Errorf("lastSeq = %d, want 0", got.LastSeq)
		}
	})

	t.Run("the 31st message in a minute is rate limited and costs no position", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)

		for range sendLimit {
			e.send(t, a, conversation.ID, "ok")
		}
		expectError(t, e.rawSend(a, conversation.ID, uuid.New(), map[string]any{"body": "too many"}), http.StatusTooManyRequests, "rate_limited")
		if got := e.conversation(t, a, conversation.ID); got.LastSeq != sendLimit {
			t.Errorf("lastSeq = %d, want %d", got.LastSeq, sendLimit)
		}
	})
}

func (e *testEnv) befriend(t *testing.T, a testUser) testUser {
	t.Helper()
	b := e.user(t)
	e.connect(t, a, b)
	return b
}

func TestMessageHistory(t *testing.T) {
	e := newTestEnv(t)

	t.Run("the cursor walks every page newest first without duplicates while messages arrive", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		for range 7 {
			e.send(t, a, conversation.ID, "old")
		}

		first := e.history(t, b, conversation.ID, "limit=3")
		if !slices.Equal(seqs(first.Items), []int64{7, 6, 5}) || first.NextCursor == nil {
			t.Fatalf("first page = %v cursor %v", seqs(first.Items), first.NextCursor)
		}
		e.send(t, a, conversation.ID, "arrived during paging")

		var all []int64
		page := first
		for {
			all = append(all, seqs(page.Items)...)
			if page.NextCursor == nil {
				break
			}
			page = e.history(t, b, conversation.ID, "limit=3&cursor="+*page.NextCursor)
		}
		if !slices.Equal(all, []int64{7, 6, 5, 4, 3, 2, 1}) {
			t.Errorf("walked %v, want 7..1 without the message that arrived later", all)
		}
	})

	t.Run("catch-up lists new messages and tombstones of older ones in change order", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		old := e.send(t, a, conversation.ID, "old")
		cursor := e.conversation(t, b, conversation.ID).ChangeSeq

		n1 := e.send(t, a, conversation.ID, "one")
		n2 := e.send(t, a, conversation.ID, "two")
		if rec := e.api.Delete("/chat/messages/"+old.ID.String(), a.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("delete: %d", rec.Code)
		}
		n3 := e.send(t, a, conversation.ID, "three")

		page := e.history(t, b, conversation.ID, "changedAfter="+strconv.FormatInt(cursor, 10))
		var ids []uuid.UUID
		var changes []int64
		for _, m := range page.Items {
			ids = append(ids, m.ID)
			changes = append(changes, m.ChangeSeq)
		}
		if !slices.Equal(ids, []uuid.UUID{n1.ID, n2.ID, old.ID, n3.ID}) {
			t.Errorf("catch-up ids = %v, want one, two, the tombstone, three", ids)
		}
		if !slices.IsSorted(changes) {
			t.Errorf("change positions %v are not ascending", changes)
		}
		tombstone := page.Items[2]
		if tombstone.DeletedAt == nil || tombstone.Body != nil || tombstone.Seq != old.Seq || tombstone.ChangeSeq <= n2.ChangeSeq {
			t.Errorf("tombstone = %+v", tombstone)
		}

		first := e.history(t, b, conversation.ID, "limit=2&changedAfter="+strconv.FormatInt(cursor, 10))
		if len(first.Items) != 2 || first.NextCursor == nil {
			t.Fatalf("first catch-up page = %d items, cursor %v", len(first.Items), first.NextCursor)
		}
		rest := e.history(t, b, conversation.ID, "limit=2&changedAfter="+strconv.FormatInt(cursor, 10)+"&cursor="+*first.NextCursor)
		if len(rest.Items) != 2 || rest.NextCursor != nil || rest.Items[0].ID != old.ID {
			t.Errorf("rest = %v cursor %v", seqs(rest.Items), rest.NextCursor)
		}
		expectErrorOnGet(t, e, b, "/chat/conversations/"+conversation.ID.String()+"/messages?changedAfter=abc", http.StatusBadRequest)
	})

	t.Run("messages of blocked and banned students never appear", func(t *testing.T) {
		owner, blockedByViewer := e.friends(t)
		banned := e.befriend(t, owner)
		viewer := e.befriend(t, owner)
		group := e.group(t, owner, "Mixed", blockedByViewer, banned, viewer)
		e.block(t, viewer, blockedByViewer)

		visible := e.send(t, owner, group.ID, "from the owner")
		e.send(t, blockedByViewer, group.ID, "from the blocked student")
		e.send(t, banned, group.ID, "from the banned student")
		if _, err := e.pool.Exec(context.Background(), "UPDATE users SET banned_at = NOW() WHERE id = $1", banned.id); err != nil {
			t.Fatal(err)
		}

		for _, query := range []string{"", "changedAfter=0"} {
			page := e.history(t, viewer, group.ID, query+"&limit=50")
			for _, m := range page.Items {
				if m.Sender.ID != nil && (*m.Sender.ID == blockedByViewer.id || *m.Sender.ID == banned.id) {
					t.Errorf("query %q shows a message from %s", query, *m.Sender.ID)
				}
			}
			if !slices.ContainsFunc(page.Items, func(m Message) bool { return m.ID == visible.ID }) {
				t.Errorf("query %q lost the owner's message", query)
			}
		}
		if rec := e.api.Get("/chat/conversations/"+group.ID.String()+"/messages?limit=50", owner.bearer); rec.Code != http.StatusOK {
			t.Fatalf("owner history: %d", rec.Code)
		}
	})
}

func expectErrorOnGet(t *testing.T, e *testEnv, u testUser, path string, status int) {
	t.Helper()
	expectError(t, e.api.Get(path, u.bearer), status, "invalid_input")
}

func TestDeleteMessage(t *testing.T) {
	e := newTestEnv(t)

	t.Run("a tombstone keeps its position, bumps the change position and releases images", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		image := e.pendingImage(t, a)
		sent := decode[Message](t, e.rawSend(a, conversation.ID, uuid.New(), map[string]any{"body": "look", "attachmentIds": []uuid.UUID{image}}), http.StatusCreated)
		if len(sent.Attachments) != 1 || sent.Attachments[0].MediaID != image || !strings.HasPrefix(sent.Attachments[0].URL, "https://media.test/") {
			t.Fatalf("attachments = %+v", sent.Attachments)
		}
		if got := e.mediaStatus(t, image); got != "ready" {
			t.Fatalf("media is %q after sending, want ready", got)
		}

		if rec := e.api.Delete("/chat/messages/"+sent.ID.String(), a.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
		}
		if rec := e.api.Delete("/chat/messages/"+sent.ID.String(), a.bearer); rec.Code != http.StatusNoContent {
			t.Errorf("deleting twice: %d, want 204", rec.Code)
		}

		got := decode[Message](t, e.api.Get("/chat/messages/"+sent.ID.String(), b.bearer), http.StatusOK)
		if got.Seq != sent.Seq || got.ChangeSeq <= sent.ChangeSeq || got.DeletedAt == nil || got.Body != nil || len(got.Attachments) != 0 || len(got.Payload) != 0 {
			t.Errorf("tombstone = %+v", got)
		}
		if got := e.mediaStatus(t, image); got != "gone" || !e.media.wasDeleted(image) {
			t.Errorf("media is %q after the delete, want released and deleted", got)
		}
		if conversation := e.conversation(t, b, conversation.ID); conversation.UnreadCount != 1 {
			t.Errorf("unread = %d, want the tombstone to stay counted", conversation.UnreadCount)
		}
	})

	t.Run("only the sender may delete, and outsiders find nothing", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		sent := e.send(t, a, conversation.ID, "mine")
		outsider := e.user(t)

		expectError(t, e.api.Delete("/chat/messages/"+sent.ID.String(), b.bearer), http.StatusForbidden, "forbidden")
		expectError(t, e.api.Delete("/chat/messages/"+sent.ID.String(), outsider.bearer), http.StatusNotFound, "not_found")
		expectError(t, e.api.Get("/chat/messages/"+sent.ID.String(), outsider.bearer), http.StatusNotFound, "not_found")
		expectError(t, e.api.Delete("/chat/messages/"+uuid.NewString(), a.bearer), http.StatusNotFound, "not_found")
	})
}

func TestMessageImages(t *testing.T) {
	e := newTestEnv(t)

	t.Run("up to five images keep their order", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		var ids []uuid.UUID
		for range maxAttachments {
			ids = append(ids, e.pendingImage(t, a))
		}

		sent := decode[Message](t, e.rawSend(a, conversation.ID, uuid.New(), map[string]any{"attachmentIds": ids}), http.StatusCreated)
		var got []uuid.UUID
		for _, att := range sent.Attachments {
			got = append(got, att.MediaID)
		}
		if !slices.Equal(got, ids) || sent.Body != nil {
			t.Errorf("attachments = %v body = %v, want %v and no body", got, sent.Body, ids)
		}
		for _, att := range sent.Attachments {
			if !att.ExpiresAt.After(e.now) {
				t.Errorf("expiresAt %v is not after now", att.ExpiresAt)
			}
		}
	})

	t.Run("six images, duplicates, foreign and sent uploads are rejected", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)

		var six []uuid.UUID
		for range maxAttachments + 1 {
			six = append(six, e.pendingImage(t, a))
		}
		expectError(t, e.rawSend(a, conversation.ID, uuid.New(), map[string]any{"attachmentIds": six}), http.StatusBadRequest, "invalid_input")

		one := e.pendingImage(t, a)
		expectError(t, e.rawSend(a, conversation.ID, uuid.New(), map[string]any{"attachmentIds": []uuid.UUID{one, one}}), http.StatusBadRequest, "invalid_input")
		expectError(t, e.rawSend(a, conversation.ID, uuid.New(), map[string]any{"attachmentIds": []uuid.UUID{e.pendingImage(t, b)}}), http.StatusBadRequest, "invalid_media")
		expectError(t, e.rawSend(a, conversation.ID, uuid.New(), map[string]any{"attachmentIds": []uuid.UUID{uuid.New()}}), http.StatusBadRequest, "invalid_media")

		decode[Message](t, e.rawSend(a, conversation.ID, uuid.New(), map[string]any{"attachmentIds": []uuid.UUID{one}}), http.StatusCreated)
		expectError(t, e.rawSend(a, conversation.ID, uuid.New(), map[string]any{"attachmentIds": []uuid.UUID{one}}), http.StatusBadRequest, "invalid_media")
		if got := e.conversation(t, a, conversation.ID); got.LastSeq != 1 {
			t.Errorf("lastSeq = %d, want only the one valid message", got.LastSeq)
		}
	})

	t.Run("a replay with the same images returns the stored message", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		image := e.pendingImage(t, a)
		key := uuid.New()
		body := map[string]any{"body": "pic", "attachmentIds": []uuid.UUID{image}}

		first := decode[Message](t, e.rawSend(a, conversation.ID, key, body), http.StatusCreated)
		rec := e.rawSend(a, conversation.ID, key, body)
		again := decode[Message](t, rec, http.StatusCreated)
		if again.ID != first.ID || rec.Header().Get("Idempotent-Replayed") != "true" {
			t.Errorf("replay = %s replayed=%q", again.ID, rec.Header().Get("Idempotent-Replayed"))
		}
		expectError(t, e.rawSend(a, conversation.ID, key, map[string]any{"body": "pic"}), http.StatusUnprocessableEntity, "idempotency_key_reused")
	})
}
