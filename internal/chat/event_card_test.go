package chat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func (e *testEnv) eventBody(startsAt time.Time) map[string]any {
	return map[string]any{"event": map[string]any{"title": " Study <i>session</i> ", "startsAt": startsAt, "place": "Library"}}
}

func (e *testEnv) rawAnswer(u testUser, messageID uuid.UUID, answer string) *httptest.ResponseRecorder {
	return e.api.Put("/chat/messages/"+messageID.String()+"/rsvp", u.bearer, map[string]any{"answer": answer})
}

func (e *testEnv) answerCount(t *testing.T, messageID uuid.UUID) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM chat_event_answers WHERE message_id = $1", messageID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *testEnv) studyGroup(t *testing.T) (testUser, testUser, testUser, Conversation) {
	t.Helper()
	owner, a := e.friends(t)
	c := e.user(t)
	e.connect(t, owner, c)
	return owner, a, c, e.group(t, owner, "Events", a, c)
}

func TestSendEvent(t *testing.T) {
	e := newTestEnv(t)

	t.Run("an event in a group has its title as body and no answers yet", func(t *testing.T) {
		owner, _, _, group := e.studyGroup(t)
		got := decode[Message](t, e.rawSend(owner, group.ID, uuid.New(), e.eventBody(e.now.Add(time.Hour))), http.StatusCreated)
		if got.Kind != MessageEvent || got.Body == nil || *got.Body != "Study session" || got.Event == nil {
			t.Fatalf("message = %+v", got)
		}
		ev := got.Event
		if ev.Title != "Study session" || ev.Place == nil || *ev.Place != "Library" || ev.Going != 0 || ev.MyAnswer != nil || ev.Ended || ev.CanceledAt != nil {
			t.Errorf("event = %+v", ev)
		}
	})

	t.Run("an event in a direct conversation is rejected", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		expectError(t, e.rawSend(a, conversation.ID, uuid.New(), e.eventBody(e.now.Add(time.Hour))), http.StatusBadRequest, "invalid_input")
	})

	t.Run("an event that starts in the past or ends before it starts is rejected", func(t *testing.T) {
		owner, _, _, group := e.studyGroup(t)
		expectError(t, e.rawSend(owner, group.ID, uuid.New(), e.eventBody(e.now.Add(-time.Minute))), http.StatusBadRequest, "invalid_input")
		body := map[string]any{"event": map[string]any{"title": "x", "startsAt": e.now.Add(time.Hour), "endsAt": e.now.Add(time.Minute)}}
		expectError(t, e.rawSend(owner, group.ID, uuid.New(), body), http.StatusBadRequest, "invalid_input")
	})
}

func TestAnswerEvent(t *testing.T) {
	e := newTestEnv(t)

	t.Run("an answer can be changed and taken back", func(t *testing.T) {
		owner, a, c, group := e.studyGroup(t)
		event := decode[Message](t, e.rawSend(owner, group.ID, uuid.New(), e.eventBody(e.now.Add(time.Hour))), http.StatusCreated)

		got := decode[Message](t, e.rawAnswer(a, event.ID, "going"), http.StatusOK)
		if got.Event.Going != 1 || got.Event.MyAnswer == nil || *got.Event.MyAnswer != AnswerGoing {
			t.Errorf("after going = %+v", got.Event)
		}
		decode[Message](t, e.rawAnswer(c, event.ID, "maybe"), http.StatusOK)
		got = decode[Message](t, e.rawAnswer(a, event.ID, "no"), http.StatusOK)
		if got.Event.Going != 0 || got.Event.Maybe != 1 || got.Event.No != 1 || len(got.Event.Attendees) != 1 {
			t.Errorf("after change = %+v", got.Event)
		}
		again := decode[Message](t, e.rawAnswer(a, event.ID, "no"), http.StatusOK)
		if again.ChangeSeq != got.ChangeSeq {
			t.Errorf("same answer moved changeSeq from %d to %d", got.ChangeSeq, again.ChangeSeq)
		}

		if rec := e.api.Delete("/chat/messages/"+event.ID.String()+"/rsvp", a.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("delete answer: %d %s", rec.Code, rec.Body.String())
		}
		if rec := e.api.Delete("/chat/messages/"+event.ID.String()+"/rsvp", a.bearer); rec.Code != http.StatusNoContent {
			t.Errorf("deleting a missing answer: %d", rec.Code)
		}
		if got := e.message(t, a, event.ID).Event; got.MyAnswer != nil || got.No != 0 || got.Maybe != 1 {
			t.Errorf("after delete = %+v", got)
		}
	})

	t.Run("a canceled event refuses answers, and only its creator or the owner may cancel it", func(t *testing.T) {
		owner, a, c, group := e.studyGroup(t)
		event := decode[Message](t, e.rawSend(a, group.ID, uuid.New(), e.eventBody(e.now.Add(time.Hour))), http.StatusCreated)
		decode[Message](t, e.rawAnswer(c, event.ID, "going"), http.StatusOK)

		expectError(t, e.api.Post("/chat/messages/"+event.ID.String()+"/event/cancel", c.bearer), http.StatusForbidden, "forbidden")
		canceled := decode[Message](t, e.api.Post("/chat/messages/"+event.ID.String()+"/event/cancel", owner.bearer), http.StatusOK)
		if canceled.Event.CanceledAt == nil {
			t.Errorf("event = %+v", canceled.Event)
		}
		decode[Message](t, e.api.Post("/chat/messages/"+event.ID.String()+"/event/cancel", a.bearer), http.StatusOK)

		expectError(t, e.rawAnswer(c, event.ID, "no"), http.StatusConflict, "event_canceled")
		if rec := e.api.Delete("/chat/messages/"+event.ID.String()+"/rsvp", c.bearer); rec.Code != http.StatusNoContent {
			t.Errorf("taking back an answer to a canceled event: %d", rec.Code)
		}
	})

	t.Run("an ended event refuses answers", func(t *testing.T) {
		env := newTestEnv(t)
		owner, a, _, group := env.studyGroup(t)
		event := decode[Message](t, env.rawSend(owner, group.ID, uuid.New(), env.eventBody(env.now.Add(time.Hour))), http.StatusCreated)
		env.now = env.now.Add(2 * time.Hour)
		expectError(t, env.rawAnswer(a, event.ID, "going"), http.StatusConflict, "event_ended")
		if !env.message(t, a, event.ID).Event.Ended {
			t.Errorf("ended = false after startsAt")
		}
	})

	t.Run("an unknown answer is rejected", func(t *testing.T) {
		owner, a, _, group := e.studyGroup(t)
		event := decode[Message](t, e.rawSend(owner, group.ID, uuid.New(), e.eventBody(e.now.Add(time.Hour))), http.StatusCreated)
		if rec := e.rawAnswer(a, event.ID, "later"); rec.Code != http.StatusUnprocessableEntity && rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want a validation error", rec.Code)
		}
	})

	t.Run("blocked students count but are not shown among attendees", func(t *testing.T) {
		owner, a, c, group := e.studyGroup(t)
		event := decode[Message](t, e.rawSend(owner, group.ID, uuid.New(), e.eventBody(e.now.Add(time.Hour))), http.StatusCreated)
		decode[Message](t, e.rawAnswer(a, event.ID, "going"), http.StatusOK)
		decode[Message](t, e.rawAnswer(c, event.ID, "going"), http.StatusOK)
		e.block(t, owner, c)

		got := e.message(t, owner, event.ID).Event
		if got.Going != 2 || len(got.Attendees) != 1 || *got.Attendees[0].ID != a.id {
			t.Errorf("event = %+v", got)
		}
	})

	t.Run("deleting an event removes its answers", func(t *testing.T) {
		owner, a, _, group := e.studyGroup(t)
		event := decode[Message](t, e.rawSend(owner, group.ID, uuid.New(), e.eventBody(e.now.Add(time.Hour))), http.StatusCreated)
		decode[Message](t, e.rawAnswer(a, event.ID, "going"), http.StatusOK)
		if rec := e.api.Delete("/chat/messages/"+event.ID.String(), owner.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("delete: %d", rec.Code)
		}
		if n := e.answerCount(t, event.ID); n != 0 {
			t.Errorf("answers = %d, want 0", n)
		}
	})

	t.Run("answering and canceling without a session return 401", func(t *testing.T) {
		id := uuid.NewString()
		for _, rec := range []*httptest.ResponseRecorder{
			e.api.Put("/chat/messages/"+id+"/rsvp", map[string]any{"answer": "going"}),
			e.api.Delete("/chat/messages/" + id + "/rsvp"),
			e.api.Post("/chat/messages/" + id + "/event/cancel"),
		} {
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
		}
	})
}
