package push

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSend(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t.Run("a direct message goes to every device of the recipient, titled with the sender's name", func(t *testing.T) {
		sender, recipient := e.user(t, "pl"), e.user(t, "pl")
		phone, tablet := newToken(), newToken()
		e.register(t, recipient, phone)
		e.register(t, e.session(t, recipient.id), tablet)
		conversationID := uuid.New()

		err := e.module.send(ctx, SendArgs{Event: EventMessage, RecipientIDs: []uuid.UUID{recipient.id}, ActorID: sender.id, ConversationID: &conversationID, Preview: "hej"})
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range []string{phone, tablet} {
			got := e.expo.to(token)
			if len(got) != 1 || got[0].Title != "Jan" || got[0].Body != "hej" ||
				got[0].Data["event"] != "message" || got[0].Data["conversationId"] != conversationID.String() {
				t.Errorf("messages to %s = %+v", token, got)
			}
		}
	})

	t.Run("a group message names the group and the sender, a photo shows as an icon", func(t *testing.T) {
		sender, recipient := e.user(t, "pl"), e.user(t, "pl")
		token := newToken()
		e.register(t, recipient, token)
		conversationID := uuid.New()
		group := "Informatyka"

		if err := e.module.send(ctx, SendArgs{Event: EventMessage, RecipientIDs: []uuid.UUID{recipient.id}, ActorID: sender.id, ConversationID: &conversationID, GroupName: &group}); err != nil {
			t.Fatal(err)
		}
		got := e.expo.to(token)
		if len(got) != 1 || got[0].Title != group || got[0].Body != "Jan: "+photoPreview {
			t.Errorf("messages = %+v", got)
		}
	})

	t.Run("a match speaks the recipient's language and points at the other student", func(t *testing.T) {
		liker, recipient := e.user(t, "pl"), e.user(t, "en")
		token := newToken()
		e.register(t, recipient, token)

		if err := e.module.send(ctx, SendArgs{Event: EventMatch, RecipientIDs: []uuid.UUID{recipient.id}, ActorID: liker.id}); err != nil {
			t.Fatal(err)
		}
		got := e.expo.to(token)
		if len(got) != 1 || got[0].Body != matchBodies["en"] || got[0].Data["userId"] != liker.id.String() {
			t.Errorf("messages = %+v", got)
		}
	})

	t.Run("students who turned notifications off or whose session ended get nothing", func(t *testing.T) {
		sender := e.user(t, "pl")
		off, expired := e.user(t, "pl"), e.user(t, "pl")
		offToken, expiredToken := newToken(), newToken()
		e.register(t, off, offToken)
		e.register(t, expired, expiredToken)
		if err := e.module.settings.setEnabled(ctx, off.id, false); err != nil {
			t.Fatal(err)
		}
		if _, err := e.pool.Exec(ctx, "UPDATE sessions SET expires_at = $2 WHERE id = $1", expired.sessionID, time.Now().Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}

		if err := e.module.send(ctx, SendArgs{Event: EventMatch, RecipientIDs: []uuid.UUID{off.id, expired.id}, ActorID: sender.id}); err != nil {
			t.Fatal(err)
		}
		if len(e.expo.to(offToken)) != 0 || len(e.expo.to(expiredToken)) != 0 {
			t.Errorf("a push went to a student who should get none")
		}
	})

	t.Run("tokens Expo no longer knows are dropped", func(t *testing.T) {
		sender, recipient := e.user(t, "pl"), e.user(t, "pl")
		token := newToken()
		e.register(t, recipient, token)
		e.expo.mu.Lock()
		e.expo.invalid[token] = true
		e.expo.mu.Unlock()

		if err := e.module.send(ctx, SendArgs{Event: EventMatch, RecipientIDs: []uuid.UUID{recipient.id}, ActorID: sender.id}); err != nil {
			t.Fatal(err)
		}
		if len(e.tokensOf(t, recipient.id)) != 0 {
			t.Errorf("unregistered token was kept")
		}
	})
}
