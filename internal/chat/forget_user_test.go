package chat

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestForgetUser(t *testing.T) {
	e := newTestEnv(t)

	t.Run("messages become tombstones, groups change hands, directs go and the user row can be deleted", func(t *testing.T) {
		a, b := e.friends(t)
		c := e.befriend(t, a)
		group := e.group(t, a, "Group", b, c)
		e.setJoinedAt(t, group.ID, c, time.Now().Add(-2*time.Hour))
		e.setJoinedAt(t, group.ID, b, time.Now().Add(-1*time.Hour))
		direct := e.direct(t, a, b)
		image := e.pendingImage(t, a)
		inGroup := decode[Message](t, e.rawSend(a, group.ID, uuid.New(), map[string]any{"body": "mine", "attachmentIds": []uuid.UUID{image}}), http.StatusCreated)
		inDirect := e.send(t, a, direct.ID, "private")
		fromB := e.send(t, b, group.ID, "from b")

		if err := e.module.ForgetUser(t.Context(), a.id); err != nil {
			t.Fatalf("ForgetUser failed: %v", err)
		}

		var deletedAt *time.Time
		var body *string
		if err := e.pool.QueryRow(t.Context(), "SELECT deleted_at, body FROM chat_messages WHERE id = $1", inGroup.ID).Scan(&deletedAt, &body); err != nil {
			t.Fatal(err)
		}
		if deletedAt == nil || body != nil {
			t.Errorf("group message: deleted_at=%v body=%v, want a tombstone", deletedAt, body)
		}
		var rows int
		if err := e.pool.QueryRow(t.Context(), "SELECT COUNT(*) FROM chat_conversations WHERE id = $1", direct.ID).Scan(&rows); err != nil || rows != 0 {
			t.Errorf("direct conversation rows = %d err = %v, want 0", rows, err)
		}
		if err := e.pool.QueryRow(t.Context(), "SELECT COUNT(*) FROM chat_messages WHERE id = $1", inDirect.ID).Scan(&rows); err != nil || rows != 0 {
			t.Errorf("direct message rows = %d err = %v, want 0", rows, err)
		}
		if got := e.mediaStatus(t, image); got != "gone" || !e.media.wasDeleted(image) {
			t.Errorf("image is %q, want released", got)
		}

		members := e.members(t, b, group.ID)
		if len(members) != 2 || members[0].Role != RoleOwner || *members[0].Student.ID != c.id {
			t.Errorf("members = %+v, want c as the new owner and a gone", members)
		}

		var before, after [2]int64
		snapshot := func(into *[2]int64) {
			if err := e.pool.QueryRow(t.Context(), "SELECT COALESCE(SUM(change_seq), 0), COUNT(*) FROM chat_messages WHERE conversation_id = $1", group.ID).Scan(&into[0], &into[1]); err != nil {
				t.Fatal(err)
			}
		}
		snapshot(&before)
		if err := e.module.ForgetUser(t.Context(), a.id); err != nil {
			t.Fatalf("second ForgetUser failed: %v", err)
		}
		snapshot(&after)
		if before != after {
			t.Errorf("a second run changed the group: %v then %v", before, after)
		}

		if _, err := e.pool.Exec(t.Context(), "DELETE FROM users WHERE id = $1", a.id); err != nil {
			t.Fatalf("the user row must be deletable after ForgetUser: %v", err)
		}
		page := e.history(t, b, group.ID, "limit=50")
		for _, m := range page.Items {
			if m.ID == inGroup.ID && (!m.Sender.Deleted || m.Sender.ID != nil || m.DeletedAt == nil) {
				t.Errorf("tombstone = %+v, want a deleted sender and deletedAt", m)
			}
			if m.ID == fromB.ID && m.DeletedAt != nil {
				t.Error("another student's message was touched")
			}
		}
	})

	t.Run("a group the user is alone in is deleted", func(t *testing.T) {
		a, b := e.friends(t)
		group := e.group(t, a, "Group", b)
		if rec := e.api.Delete("/chat/conversations/"+group.ID.String()+"/members/me", b.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("leave: %d", rec.Code)
		}

		if err := e.module.ForgetUser(t.Context(), a.id); err != nil {
			t.Fatalf("ForgetUser failed: %v", err)
		}
		var rows int
		if err := e.pool.QueryRow(t.Context(), "SELECT COUNT(*) FROM chat_conversations WHERE id = $1", group.ID).Scan(&rows); err != nil || rows != 0 {
			t.Errorf("group rows = %d err = %v, want 0", rows, err)
		}
	})
}
