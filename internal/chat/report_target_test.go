package chat

import (
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestReportTarget(t *testing.T) {
	e := newTestEnv(t)

	t.Run("a member can report a message and gets its text without the messages around it", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		e.send(t, a, conversation.ID, "before")
		image := e.pendingImage(t, a)
		reported := decode[Message](t, e.rawSend(a, conversation.ID, uuid.New(), map[string]any{"body": "the reported text", "attachmentIds": []uuid.UUID{image}}), http.StatusCreated)
		e.send(t, a, conversation.ID, "after")

		authorID, snapshot, err := e.module.ReportTarget(t.Context(), b.id, reported.ID)
		if err != nil {
			t.Fatalf("ReportTarget failed: %v", err)
		}
		if authorID != a.id {
			t.Errorf("author = %s, want %s", authorID, a.id)
		}
		if snapshot["text"] != "the reported text" || snapshot["kind"] != MessageText || snapshot["attachments"] != 1 || len(snapshot) != 3 {
			t.Errorf("snapshot = %v, want text, kind and attachment count only", snapshot)
		}
	})

	t.Run("outsiders, blocked students, deleted and system messages are not found", func(t *testing.T) {
		owner, b := e.friends(t)
		c := e.befriend(t, owner)
		group := e.group(t, owner, "Group", b, c)
		sent := e.send(t, c, group.ID, "text")
		outsider := e.user(t)
		e.block(t, b, c)

		if _, _, err := e.module.ReportTarget(t.Context(), outsider.id, sent.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("outsider: %v, want ErrNotFound", err)
		}
		if _, _, err := e.module.ReportTarget(t.Context(), b.id, sent.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("blocked sender: %v, want ErrNotFound", err)
		}
		if _, _, err := e.module.ReportTarget(t.Context(), owner.id, sent.ID); err != nil {
			t.Errorf("owner: %v, want the message", err)
		}

		if rec := e.api.Delete("/chat/messages/"+sent.ID.String(), c.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("delete: %d", rec.Code)
		}
		if _, _, err := e.module.ReportTarget(t.Context(), owner.id, sent.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("deleted message: %v, want ErrNotFound", err)
		}

		system := e.history(t, owner, group.ID, "limit=50").Items
		last := system[len(system)-1]
		if last.Kind != MessageSystem {
			t.Fatalf("oldest message is %s, want the system message", last.Kind)
		}
		if _, _, err := e.module.ReportTarget(t.Context(), b.id, last.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("system message: %v, want ErrNotFound", err)
		}
	})
}
