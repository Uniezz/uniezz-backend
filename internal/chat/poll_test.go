package chat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func pollBody(options ...string) map[string]any {
	return map[string]any{"poll": pollFields(options...)}
}

func pollFields(options ...string) map[string]any {
	list := make([]map[string]any, 0, len(options))
	for _, o := range options {
		list = append(list, map[string]any{"text": o})
	}
	return map[string]any{"question": "Where do we study?", "options": list}
}

func (e *testEnv) sendPoll(t *testing.T, u testUser, conversationID uuid.UUID, body map[string]any) Message {
	t.Helper()
	return decode[Message](t, e.rawSend(u, conversationID, uuid.New(), body), http.StatusCreated)
}

func (e *testEnv) rawVote(u testUser, messageID uuid.UUID, ids ...int) *httptest.ResponseRecorder {
	if ids == nil {
		ids = []int{}
	}
	return e.api.Put("/chat/messages/"+messageID.String()+"/poll/vote", u.bearer, map[string]any{"optionIds": ids})
}

func (e *testEnv) vote(t *testing.T, u testUser, messageID uuid.UUID, ids ...int) Message {
	t.Helper()
	return decode[Message](t, e.rawVote(u, messageID, ids...), http.StatusOK)
}

func (e *testEnv) votes(t *testing.T, messageID, userID uuid.UUID) []int {
	t.Helper()
	rows, err := e.pool.Query(context.Background(), "SELECT option_id FROM chat_poll_votes WHERE message_id = $1 AND user_id = $2 ORDER BY option_id", messageID, userID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func optionVotes(p *Poll) []int {
	out := make([]int, 0, len(p.Options))
	for _, o := range p.Options {
		out = append(out, o.Votes)
	}
	return out
}

func TestSendPoll(t *testing.T) {
	e := newTestEnv(t)

	t.Run("a poll gets option ids in order and the question as its body", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)

		got := e.sendPoll(t, a, conversation.ID, pollBody(" Library ", "Cafe"))
		if got.Kind != MessagePoll || got.Body == nil || *got.Body != "Where do we study?" || got.Poll == nil {
			t.Fatalf("message = %+v", got)
		}
		p := got.Poll
		if len(p.Options) != 2 || p.Options[0].ID != 1 || p.Options[0].Text != "Library" || p.Options[1].ID != 2 || p.Closed || p.TotalVoters != 0 {
			t.Errorf("poll = %+v", p)
		}
	})

	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{"one option", pollBody("Library")},
		{"eleven options", pollBody("1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11")},
		{"an empty option", pollBody("Library", " <b></b> ")},
		{"a body next to the poll", func() map[string]any { b := pollBody("a", "b"); b["body"] = "hi"; return b }()},
	} {
		t.Run("a poll with "+tc.name+" is rejected", func(t *testing.T) {
			a, b := e.friends(t)
			conversation := e.direct(t, a, b)
			expectError(t, e.rawSend(a, conversation.ID, uuid.New(), tc.body), http.StatusBadRequest, "invalid_input")
		})
	}

	t.Run("closesAt must be in the future and at most 30 days ahead", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		for _, at := range []time.Time{e.now.Add(-time.Minute), e.now.Add(31 * 24 * time.Hour)} {
			fields := pollFields("a", "b")
			fields["closesAt"] = at
			expectError(t, e.rawSend(a, conversation.ID, uuid.New(), map[string]any{"poll": fields}), http.StatusBadRequest, "invalid_input")
		}
	})

	t.Run("repeating the request after a vote returns the stored poll", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		key := uuid.New()
		first := decode[Message](t, e.rawSend(a, conversation.ID, key, pollBody("a", "b")), http.StatusCreated)
		e.vote(t, b, first.ID, 1)

		again := decode[Message](t, e.rawSend(a, conversation.ID, key, pollBody("a", "b")), http.StatusCreated)
		if again.ID != first.ID {
			t.Errorf("replayed id = %s, want %s", again.ID, first.ID)
		}
		expectError(t, e.rawSend(a, conversation.ID, key, pollBody("a", "c")), http.StatusUnprocessableEntity, "idempotency_key_reused")
	})

	t.Run("a pending message request cannot be a poll", func(t *testing.T) {
		a, b := e.user(t), e.user(t)
		rec := e.api.Post("/chat/requests", a.bearer, keyHeader(uuid.New()), map[string]any{"userId": b.id, "body": "hi"})
		conversation := decode[Conversation](t, rec, http.StatusCreated)
		expectError(t, e.rawSend(a, conversation.ID, uuid.New(), pollBody("a", "b")), http.StatusConflict, "request_pending")
	})
}

func TestVotePoll(t *testing.T) {
	e := newTestEnv(t)

	t.Run("a vote can be changed and taken back while the poll is open", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		poll := e.sendPoll(t, a, conversation.ID, pollBody("a", "b", "c"))

		got := e.vote(t, b, poll.ID, 2)
		if !slices.Equal(got.Poll.MyVote, []int{2}) || !slices.Equal(optionVotes(got.Poll), []int{0, 1, 0}) || got.Poll.TotalVoters != 1 {
			t.Errorf("after vote = %+v", got.Poll)
		}
		got = e.vote(t, b, poll.ID, 3)
		if !slices.Equal(optionVotes(got.Poll), []int{0, 0, 1}) || !slices.Equal(e.votes(t, poll.ID, b.id), []int{3}) {
			t.Errorf("after change = %+v", got.Poll)
		}
		got = e.vote(t, b, poll.ID)
		if len(got.Poll.MyVote) != 0 || got.Poll.TotalVoters != 0 || len(e.votes(t, poll.ID, b.id)) != 0 {
			t.Errorf("after retract = %+v", got.Poll)
		}
	})

	t.Run("sending the current vote again changes nothing", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		poll := e.sendPoll(t, a, conversation.ID, pollBody("a", "b"))
		first := e.vote(t, b, poll.ID, 1)
		again := e.vote(t, b, poll.ID, 1)
		if again.ChangeSeq != first.ChangeSeq {
			t.Errorf("changeSeq moved from %d to %d", first.ChangeSeq, again.ChangeSeq)
		}
	})

	t.Run("a multiple choice poll takes several options, a single choice poll only one", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		single := e.sendPoll(t, a, conversation.ID, pollBody("a", "b"))
		fields := pollFields("a", "b", "c")
		fields["multipleChoice"] = true
		multiple := e.sendPoll(t, a, conversation.ID, map[string]any{"poll": fields})

		expectError(t, e.rawVote(b, single.ID, 1, 2), http.StatusBadRequest, "invalid_input")
		expectError(t, e.rawVote(b, single.ID, 3), http.StatusBadRequest, "invalid_input")
		got := e.vote(t, b, multiple.ID, 3, 1)
		if !slices.Equal(got.Poll.MyVote, []int{1, 3}) || got.Poll.TotalVoters != 1 {
			t.Errorf("multiple = %+v", got.Poll)
		}
	})

	t.Run("a closed poll refuses votes, and only its creator or the group owner may close it", func(t *testing.T) {
		owner, a := e.friends(t)
		c := e.user(t)
		e.connect(t, owner, c)
		group := e.group(t, owner, "Poll", a, c)
		poll := e.sendPoll(t, a, group.ID, pollBody("a", "b"))
		e.vote(t, c, poll.ID, 1)

		expectError(t, e.api.Post("/chat/messages/"+poll.ID.String()+"/poll/close", c.bearer), http.StatusForbidden, "forbidden")
		closed := decode[Message](t, e.api.Post("/chat/messages/"+poll.ID.String()+"/poll/close", owner.bearer), http.StatusOK)
		if !closed.Poll.Closed || closed.Poll.ClosedAt == nil {
			t.Errorf("poll = %+v", closed.Poll)
		}
		decode[Message](t, e.api.Post("/chat/messages/"+poll.ID.String()+"/poll/close", a.bearer), http.StatusOK)

		expectError(t, e.rawVote(c, poll.ID, 2), http.StatusConflict, "poll_closed")
		expectError(t, e.rawVote(c, poll.ID), http.StatusConflict, "poll_closed")
		if got := e.votes(t, poll.ID, c.id); !slices.Equal(got, []int{1}) {
			t.Errorf("votes = %v, want [1]", got)
		}
	})

	t.Run("a poll past closesAt is closed", func(t *testing.T) {
		env := newTestEnv(t)
		a, b := env.friends(t)
		conversation := env.direct(t, a, b)
		fields := pollFields("a", "b")
		fields["closesAt"] = env.now.Add(time.Hour)
		poll := env.sendPoll(t, a, conversation.ID, map[string]any{"poll": fields})
		env.vote(t, b, poll.ID, 1)

		env.now = env.now.Add(2 * time.Hour)
		expectError(t, env.rawVote(b, poll.ID, 2), http.StatusConflict, "poll_closed")
		if got := env.message(t, b, poll.ID); !got.Poll.Closed {
			t.Errorf("closed = false after closesAt")
		}
	})

	t.Run("concurrent votes from one student leave exactly one choice", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		poll := e.sendPoll(t, a, conversation.ID, pollBody("a", "b", "c"))

		var wg sync.WaitGroup
		for i := range 9 {
			wg.Go(func() {
				rec := e.rawVote(b, poll.ID, i%3+1)
				if rec.Code != http.StatusOK {
					t.Errorf("vote: %d %s", rec.Code, rec.Body.String())
				}
			})
		}
		wg.Wait()
		if got := e.votes(t, poll.ID, b.id); len(got) != 1 {
			t.Errorf("votes = %v, want one", got)
		}
	})

	t.Run("anonymous polls never list voters, public polls leave out blocked voters but count them", func(t *testing.T) {
		owner, a := e.friends(t)
		c := e.user(t)
		e.connect(t, owner, c)
		group := e.group(t, owner, "Votes", a, c)
		public := e.sendPoll(t, owner, group.ID, pollBody("a", "b"))
		fields := pollFields("a", "b")
		fields["anonymous"] = true
		anonymous := e.sendPoll(t, owner, group.ID, map[string]any{"poll": fields})
		for _, voter := range []testUser{a, c} {
			e.vote(t, voter, public.ID, 1)
			e.vote(t, voter, anonymous.ID, 1)
		}
		e.block(t, owner, c)

		got := e.message(t, owner, public.ID).Poll
		if got.TotalVoters != 2 || len(got.Voters) != 1 || got.Voters[0].ID == nil || *got.Voters[0].ID != a.id {
			t.Errorf("public poll = %+v", got)
		}
		if got := e.message(t, owner, anonymous.ID).Poll; got.TotalVoters != 2 || len(got.Voters) != 0 {
			t.Errorf("anonymous poll = %+v", got)
		}
	})

	t.Run("deleting a poll removes its votes", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		poll := e.sendPoll(t, a, conversation.ID, pollBody("a", "b"))
		e.vote(t, b, poll.ID, 1)
		if rec := e.api.Delete("/chat/messages/"+poll.ID.String(), a.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
		}
		if got := e.votes(t, poll.ID, b.id); len(got) != 0 {
			t.Errorf("votes = %v, want none", got)
		}
		expectError(t, e.rawVote(b, poll.ID, 1), http.StatusNotFound, "not_found")
	})

	t.Run("a text message is not a poll", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		m := e.send(t, a, conversation.ID, "hello")
		expectError(t, e.rawVote(b, m.ID, 1), http.StatusNotFound, "not_found")
	})

	t.Run("a student outside the conversation cannot vote", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		poll := e.sendPoll(t, a, conversation.ID, pollBody("a", "b"))
		expectError(t, e.rawVote(e.user(t), poll.ID, 1), http.StatusNotFound, "not_found")
	})

	t.Run("voting and closing without a session return 401", func(t *testing.T) {
		id := uuid.NewString()
		if rec := e.api.Put("/chat/messages/"+id+"/poll/vote", map[string]any{"optionIds": []int{1}}); rec.Code != http.StatusUnauthorized {
			t.Errorf("vote status = %d", rec.Code)
		}
		if rec := e.api.Post("/chat/messages/" + id + "/poll/close"); rec.Code != http.StatusUnauthorized {
			t.Errorf("close status = %d", rec.Code)
		}
	})
}
