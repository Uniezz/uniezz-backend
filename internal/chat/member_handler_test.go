package chat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type memberPage struct {
	Items      []Member
	NextCursor *string
}

func (e *testEnv) members(t *testing.T, u testUser, conversationID uuid.UUID) []Member {
	t.Helper()
	rec := e.api.Get("/chat/conversations/"+conversationID.String()+"/members?limit=50", u.bearer)
	return decode[memberPage](t, rec, http.StatusOK).Items
}

func systemEvents(messages []Message) []string {
	var events []string
	for _, m := range messages {
		if m.Kind == MessageSystem {
			event, _ := m.Payload["event"].(string)
			events = append(events, event)
		}
	}
	return events
}

func (e *testEnv) setJoinedAt(t *testing.T, conversationID uuid.UUID, u testUser, joinedAt time.Time) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), "UPDATE chat_members SET joined_at = $3 WHERE conversation_id = $1 AND user_id = $2", conversationID, u.id, joinedAt); err != nil {
		t.Fatal(err)
	}
}

func TestCreateGroup(t *testing.T) {
	e := newTestEnv(t)

	t.Run("the creator owns the group and every member starts with nothing unread", func(t *testing.T) {
		owner, b := e.friends(t)
		c := e.befriend(t, owner)

		group := e.group(t, owner, " <i>Informatyka</i> ", b, c)
		if group.Kind != KindGroup || group.Name == nil || *group.Name != "Informatyka" {
			t.Fatalf("group = %+v", group)
		}
		if group.MemberCount == nil || *group.MemberCount != 3 || group.MyRole == nil || *group.MyRole != RoleOwner || group.Peer != nil {
			t.Errorf("group fields = %+v", group)
		}
		if group.LastSeq != 1 || group.LastMessage == nil || group.LastMessage.Payload["event"] != "group_created" {
			t.Errorf("group_created is missing: %+v", group.LastMessage)
		}

		got := e.conversation(t, b, group.ID)
		if got.UnreadCount != 0 || got.LastReadSeq != 1 || got.MyRole == nil || *got.MyRole != RoleMember {
			t.Errorf("member view = %+v", got)
		}
		members := e.members(t, b, group.ID)
		if len(members) != 3 || members[0].Role != RoleOwner || members[0].Student.ID == nil || *members[0].Student.ID != owner.id {
			t.Errorf("members = %+v, want the owner first", members)
		}
	})

	t.Run("members must be the creator's own unblocked connections", func(t *testing.T) {
		owner, friend := e.friends(t)
		stranger := e.user(t)
		blocked := e.befriend(t, owner)
		e.block(t, owner, blocked)
		create := func(name string, ids ...uuid.UUID) *httptest.ResponseRecorder {
			return e.api.Post("/chat/conversations/groups", owner.bearer, map[string]any{"name": name, "memberIds": ids})
		}

		expectError(t, create("Bad", stranger.id), http.StatusBadRequest, "invalid_member")
		expectError(t, create("Bad", friend.id, blocked.id), http.StatusBadRequest, "invalid_member")
		expectError(t, create("Bad", owner.id), http.StatusBadRequest, "invalid_input")
		expectError(t, create("Bad", friend.id, friend.id), http.StatusBadRequest, "invalid_input")
		expectError(t, create("Bad"), http.StatusBadRequest, "invalid_input")
		expectError(t, create("  ", friend.id), http.StatusBadRequest, "invalid_input")
	})

	t.Run("a retry with the same key returns the group and another input is rejected", func(t *testing.T) {
		owner, b := e.friends(t)
		c := e.befriend(t, owner)
		key := uuid.New()
		body := map[string]any{"name": "Once", "memberIds": []uuid.UUID{b.id}}

		first := decode[Conversation](t, e.api.Post("/chat/conversations/groups", owner.bearer, keyHeader(key), body), http.StatusCreated)
		rec := e.api.Post("/chat/conversations/groups", owner.bearer, keyHeader(key), body)
		again := decode[Conversation](t, rec, http.StatusCreated)
		if again.ID != first.ID || rec.Header().Get("Idempotent-Replayed") != "true" {
			t.Errorf("replay = %s replayed=%q", again.ID, rec.Header().Get("Idempotent-Replayed"))
		}
		expectError(t, e.api.Post("/chat/conversations/groups", owner.bearer, keyHeader(key), map[string]any{"name": "Once", "memberIds": []uuid.UUID{c.id}}), http.StatusUnprocessableEntity, "idempotency_key_reused")

		var rows int
		err := e.pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM chat_messages WHERE client_id = $1", key).Scan(&rows)
		if err != nil || rows != 1 {
			t.Errorf("rows = %d err = %v, want 1", rows, err)
		}
	})

	t.Run("the sixth group within an hour is rate limited", func(t *testing.T) {
		owner, b := e.friends(t)
		for range groupLimit {
			e.group(t, owner, "Group", b)
		}
		expectError(t, e.api.Post("/chat/conversations/groups", owner.bearer, map[string]any{"name": "Group", "memberIds": []uuid.UUID{b.id}}), http.StatusTooManyRequests, "rate_limited")
	})
}

func TestGroupMembers(t *testing.T) {
	e := newTestEnv(t)

	t.Run("adding records a system message and the new member sees nothing unread", func(t *testing.T) {
		owner, b := e.friends(t)
		group := e.group(t, owner, "Group", b)
		newcomer := e.befriend(t, b)
		url := "/chat/conversations/" + group.ID.String() + "/members"

		added := decode[struct{ Items []Member }](t, e.api.Post(url, b.bearer, map[string]any{"userIds": []uuid.UUID{newcomer.id, b.id}}), http.StatusCreated)
		if len(added.Items) != 1 || added.Items[0].Student.ID == nil || *added.Items[0].Student.ID != newcomer.id {
			t.Fatalf("added = %+v, want only the newcomer", added.Items)
		}
		again := decode[struct{ Items []Member }](t, e.api.Post(url, b.bearer, map[string]any{"userIds": []uuid.UUID{newcomer.id}}), http.StatusCreated)
		if len(again.Items) != 0 {
			t.Errorf("adding a member twice added %d", len(again.Items))
		}

		if got := e.conversation(t, newcomer, group.ID); got.UnreadCount != 0 || got.LastSeq != 2 {
			t.Errorf("newcomer view = %+v, want lastSeq 2 and nothing unread", got)
		}
		if got := e.conversation(t, owner, group.ID); got.UnreadCount != 1 {
			t.Errorf("owner unread = %d, want the member_added message", got.UnreadCount)
		}
		events := systemEvents(e.history(t, owner, group.ID, "limit=50").Items)
		if !slices.Equal(events, []string{"member_added", "group_created"}) {
			t.Errorf("events = %v", events)
		}
	})

	t.Run("a group holds at most 50 members even under concurrent adds", func(t *testing.T) {
		owner := e.user(t)
		first := make([]testUser, 0, maxGroupMembers-3)
		for range maxGroupMembers - 3 {
			first = append(first, e.befriend(t, owner))
		}
		group := e.group(t, owner, "Big", first...)
		extra := []testUser{e.befriend(t, owner), e.befriend(t, owner), e.befriend(t, owner), e.befriend(t, owner)}

		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for i := range 2 {
			wg.Go(func() {
				rec := e.api.Post("/chat/conversations/"+group.ID.String()+"/members", owner.bearer,
					map[string]any{"userIds": []uuid.UUID{extra[2*i].id, extra[2*i+1].id}})
				codes <- rec.Code
			})
		}
		wg.Wait()
		close(codes)

		var got []int
		for c := range codes {
			got = append(got, c)
		}
		slices.Sort(got)
		if !slices.Equal(got, []int{http.StatusCreated, http.StatusConflict}) {
			t.Errorf("statuses = %v, want one 201 and one 409", got)
		}
		var count int
		if err := e.pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM chat_members WHERE conversation_id = $1", group.ID).Scan(&count); err != nil || count != maxGroupMembers {
			t.Errorf("members = %d err = %v, want %d", count, err, maxGroupMembers)
		}
		latecomer := e.befriend(t, owner)
		expectError(t, e.api.Post("/chat/conversations/"+group.ID.String()+"/members", owner.bearer, map[string]any{"userIds": []uuid.UUID{latecomer.id}}), http.StatusConflict, "group_full")
	})

	t.Run("only the owner renames or removes", func(t *testing.T) {
		owner, b := e.friends(t)
		c := e.befriend(t, owner)
		group := e.group(t, owner, "Group", b, c)
		url := "/chat/conversations/" + group.ID.String()

		expectError(t, e.api.Patch(url, b.bearer, map[string]any{"name": "Mine"}), http.StatusForbidden, "forbidden")
		expectError(t, e.api.Delete(url+"/members/"+c.id.String(), b.bearer), http.StatusForbidden, "forbidden")
		expectError(t, e.api.Patch(url, e.user(t).bearer, map[string]any{"name": "Mine"}), http.StatusNotFound, "not_found")
		expectError(t, e.api.Patch(url, owner.bearer, map[string]any{"name": " "}), http.StatusBadRequest, "invalid_input")

		renamed := decode[Conversation](t, e.api.Patch(url, owner.bearer, map[string]any{"name": " <b>New</b> name "}), http.StatusOK)
		if renamed.Name == nil || *renamed.Name != "New name" || renamed.LastSeq != 2 {
			t.Errorf("renamed = %+v", renamed)
		}
		same := decode[Conversation](t, e.api.Patch(url, owner.bearer, map[string]any{"name": "New name"}), http.StatusOK)
		if same.LastSeq != 2 {
			t.Errorf("sending the current name added a message: lastSeq %d", same.LastSeq)
		}
		expectError(t, e.api.Delete(url+"/members/"+owner.id.String(), owner.bearer), http.StatusBadRequest, "invalid_input")

		if rec := e.api.Delete(url+"/members/"+c.id.String(), owner.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("remove: %d %s", rec.Code, rec.Body.String())
		}
		expectError(t, e.api.Get(url, c.bearer), http.StatusNotFound, "not_found")
		expectError(t, e.api.Delete(url+"/members/"+c.id.String(), owner.bearer), http.StatusNotFound, "not_found")
		events := systemEvents(e.history(t, owner, group.ID, "limit=50").Items)
		if !slices.Equal(events, []string{"member_removed", "renamed", "group_created"}) {
			t.Errorf("events = %v", events)
		}
	})

	t.Run("a member cannot be removed from a direct conversation or read someone else's group", func(t *testing.T) {
		a, b := e.friends(t)
		direct := e.direct(t, a, b)
		expectError(t, e.api.Delete("/chat/conversations/"+direct.ID.String()+"/members/me", a.bearer), http.StatusBadRequest, "invalid_input")
		expectError(t, e.api.Get("/chat/conversations/"+direct.ID.String()+"/members", e.user(t).bearer), http.StatusNotFound, "not_found")
	})
}

func TestLeaveGroup(t *testing.T) {
	e := newTestEnv(t)

	t.Run("the longest-standing member takes over when the owner leaves", func(t *testing.T) {
		owner, b := e.friends(t)
		c := e.befriend(t, owner)
		group := e.group(t, owner, "Group", b, c)
		e.setJoinedAt(t, group.ID, c, time.Now().Add(-2*time.Hour))
		e.setJoinedAt(t, group.ID, b, time.Now().Add(-1*time.Hour))

		if rec := e.api.Delete("/chat/conversations/"+group.ID.String()+"/members/me", owner.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("leave: %d %s", rec.Code, rec.Body.String())
		}
		expectError(t, e.api.Get("/chat/conversations/"+group.ID.String(), owner.bearer), http.StatusNotFound, "not_found")

		members := e.members(t, b, group.ID)
		if len(members) != 2 || members[0].Role != RoleOwner || *members[0].Student.ID != c.id {
			t.Errorf("members = %+v, want c as the new owner", members)
		}
		page := e.history(t, b, group.ID, "limit=50")
		if events := systemEvents(page.Items); !slices.Equal(events, []string{"owner_changed", "member_left", "group_created"}) {
			t.Errorf("events = %v", events)
		}
		if got := e.conversation(t, c, group.ID); got.MyRole == nil || *got.MyRole != RoleOwner {
			t.Errorf("c's role = %v", got.MyRole)
		}
	})

	t.Run("the last member leaving deletes the group and releases its images", func(t *testing.T) {
		owner, b := e.friends(t)
		group := e.group(t, owner, "Group", b)
		image := e.pendingImage(t, owner)
		decode[Message](t, e.rawSend(owner, group.ID, uuid.New(), map[string]any{"attachmentIds": []uuid.UUID{image}}), http.StatusCreated)

		for _, u := range []testUser{owner, b} {
			if rec := e.api.Delete("/chat/conversations/"+group.ID.String()+"/members/me", u.bearer); rec.Code != http.StatusNoContent {
				t.Fatalf("leave: %d %s", rec.Code, rec.Body.String())
			}
		}

		var rows int
		err := e.pool.QueryRow(context.Background(), `
			SELECT (SELECT COUNT(*) FROM chat_conversations WHERE id = $1) + (SELECT COUNT(*) FROM chat_messages WHERE conversation_id = $1)
			     + (SELECT COUNT(*) FROM chat_message_media WHERE media_id = $2)`, group.ID, image).Scan(&rows)
		if err != nil || rows != 0 {
			t.Errorf("rows left = %d err = %v, want 0", rows, err)
		}
		if got := e.mediaStatus(t, image); got != "gone" || !e.media.wasDeleted(image) {
			t.Errorf("image is %q, want released and deleted", got)
		}
	})
}

func TestBlocksInsideGroups(t *testing.T) {
	e := newTestEnv(t)
	owner, b := e.friends(t)
	c := e.befriend(t, owner)
	group := e.group(t, owner, "Group", b, c)
	e.block(t, b, c)

	e.send(t, owner, group.ID, "from the owner")
	e.send(t, c, group.ID, "from c")

	forB := e.conversation(t, b, group.ID)
	if forB.UnreadCount != 1 {
		t.Errorf("b unread = %d, want only the owner's message", forB.UnreadCount)
	}
	if forB.LastMessage == nil || forB.LastMessage.Body == nil || *forB.LastMessage.Body != "from the owner" {
		t.Errorf("b last message = %+v", forB.LastMessage)
	}
	if got := e.conversation(t, owner, group.ID).UnreadCount; got != 1 {
		t.Errorf("owner unread = %d, want c's message", got)
	}
	for _, m := range e.members(t, b, group.ID) {
		if m.Student.ID != nil && *m.Student.ID == c.id {
			t.Error("b still sees c in the member list")
		}
	}
	for _, m := range e.members(t, c, group.ID) {
		if m.Student.ID != nil && *m.Student.ID == b.id {
			t.Error("c still sees b in the member list")
		}
	}
	if len(e.members(t, owner, group.ID)) != 3 {
		t.Error("the owner must still see everyone")
	}
}

func TestReadPositions(t *testing.T) {
	e := newTestEnv(t)

	read := func(u testUser, conversationID uuid.UUID, seq int64) *httptest.ResponseRecorder {
		return e.api.Put("/chat/conversations/"+conversationID.String()+"/read", u.bearer, map[string]any{"seq": seq})
	}
	type readResult struct {
		LastReadSeq int64
		UnreadCount int
	}

	t.Run("the position only moves forward and recounts what is unread", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		for range 5 {
			e.send(t, a, conversation.ID, "hi")
		}
		if got := e.conversation(t, b, conversation.ID); got.UnreadCount != 5 || got.LastReadSeq != 0 {
			t.Fatalf("before reading = %+v", got)
		}
		if got := e.conversation(t, a, conversation.ID); got.UnreadCount != 0 || got.LastReadSeq != 5 {
			t.Errorf("the sender's own messages count: %+v", got)
		}

		if got := decode[readResult](t, read(b, conversation.ID, 3), http.StatusOK); got.LastReadSeq != 3 || got.UnreadCount != 2 {
			t.Errorf("read 3 = %+v", got)
		}
		if got := decode[readResult](t, read(b, conversation.ID, 1), http.StatusOK); got.LastReadSeq != 3 || got.UnreadCount != 2 {
			t.Errorf("a stale read moved the position: %+v", got)
		}
		if got := decode[readResult](t, read(b, conversation.ID, 5), http.StatusOK); got.LastReadSeq != 5 || got.UnreadCount != 0 {
			t.Errorf("read 5 = %+v", got)
		}
		expectError(t, read(b, conversation.ID, 6), http.StatusBadRequest, "invalid_input")
		expectError(t, read(e.user(t), conversation.ID, 0), http.StatusNotFound, "not_found")
	})

	t.Run("totals add up over the visible conversations only", func(t *testing.T) {
		me := e.user(t)
		var friends []testUser
		for range 3 {
			friend := e.befriend(t, me)
			friends = append(friends, friend)
			c := e.direct(t, friend, me)
			e.send(t, friend, c.ID, "one")
			e.send(t, friend, c.ID, "two")
		}

		got := decode[struct {
			TotalUnread         int
			UnreadConversations int
		}](t, e.api.Get("/chat/unread", me.bearer), http.StatusOK)
		if got.TotalUnread != 6 || got.UnreadConversations != 3 {
			t.Errorf("totals = %+v, want 6 and 3", got)
		}

		e.block(t, me, friends[0])
		got = decode[struct {
			TotalUnread         int
			UnreadConversations int
		}](t, e.api.Get("/chat/unread", me.bearer), http.StatusOK)
		if got.TotalUnread != 4 || got.UnreadConversations != 2 {
			t.Errorf("totals with a block = %+v, want 4 and 2", got)
		}
	})

	t.Run("concurrent reads and sends keep the stored counter equal to the raw count", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)

		var wg sync.WaitGroup
		for i := range 10 {
			wg.Go(func() { e.send(t, a, conversation.ID, "m"+strconv.Itoa(i)) })
			wg.Go(func() { read(b, conversation.ID, 0) })
		}
		wg.Wait()
		decode[readResult](t, read(b, conversation.ID, 4), http.StatusOK)

		var stored, raw int
		err := e.pool.QueryRow(context.Background(), `
			SELECT m.unread_count, (SELECT COUNT(*) FROM chat_messages x WHERE x.conversation_id = m.conversation_id AND x.seq > m.last_read_seq AND x.sender_id <> m.user_id)
			FROM chat_members m WHERE m.conversation_id = $1 AND m.user_id = $2`, conversation.ID, b.id).Scan(&stored, &raw)
		if err != nil {
			t.Fatal(err)
		}
		if stored != raw {
			t.Errorf("unread_count = %d, raw count = %d", stored, raw)
		}
	})
}
