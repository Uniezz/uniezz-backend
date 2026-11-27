package chat

import (
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/google/uuid"
)

func (e *testEnv) search(t *testing.T, u testUser, conversationID uuid.UUID, q, cursor string) messagePage {
	t.Helper()
	query := url.Values{"q": {q}, "limit": {"2"}}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	rec := e.api.Get("/chat/conversations/"+conversationID.String()+"/messages/search?"+query.Encode(), u.bearer)
	return decode[messagePage](t, rec, http.StatusOK)
}

func TestSearchMessages(t *testing.T) {
	e := newTestEnv(t)

	t.Run("every word must match, ignoring case and accents, newest first across pages", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		first := e.send(t, a, conversation.ID, "Egzamin z ANALIZY w sali 105")
		e.send(t, b, conversation.ID, "egzamin jutro")
		second := e.send(t, b, conversation.ID, "Gdzie jest egzamin z analizy?")
		third := e.send(t, a, conversation.ID, "Analíza i egzámin")

		page := e.search(t, a, conversation.ID, "egzamin analiz", "")
		if got := seqs(page.Items); !slices.Equal(got, []int64{third.Seq, second.Seq}) || page.NextCursor == nil {
			t.Fatalf("first page = %v, cursor %v", got, page.NextCursor)
		}
		page = e.search(t, a, conversation.ID, "egzamin analiz", *page.NextCursor)
		if got := seqs(page.Items); !slices.Equal(got, []int64{first.Seq}) || page.NextCursor != nil {
			t.Errorf("second page = %v, cursor %v", got, page.NextCursor)
		}
	})

	t.Run("deleted messages and messages from a blocked member are never returned", func(t *testing.T) {
		owner, a := e.friends(t)
		c := e.user(t)
		e.connect(t, owner, c)
		group := e.group(t, owner, "Study", a, c)
		kept := e.send(t, a, group.ID, "kolokwium w piątek")
		deleted := e.send(t, a, group.ID, "kolokwium przeniesione")
		e.send(t, c, group.ID, "kolokwium odwołane")
		e.block(t, owner, c)

		rec := e.api.Delete("/chat/messages/"+deleted.ID.String(), a.bearer)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
		}

		page := e.search(t, owner, group.ID, "kolokwium", "")
		if got := seqs(page.Items); !slices.Equal(got, []int64{kept.Seq}) {
			t.Errorf("matches = %v, want only %d", got, kept.Seq)
		}
	})

	t.Run("matches from other conversations are left out", func(t *testing.T) {
		a, b := e.friends(t)
		mine := e.direct(t, a, b)
		other, c := e.friends(t)
		theirs := e.direct(t, other, c)
		marker := "marker" + uuid.NewString()[:8]
		e.send(t, other, theirs.ID, marker)

		if page := e.search(t, a, mine.ID, marker, ""); len(page.Items) != 0 {
			t.Errorf("got %d matches from another conversation", len(page.Items))
		}
	})

	t.Run("a word shorter than 3 characters is rejected", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		rec := e.api.Get("/chat/conversations/"+conversation.ID.String()+"/messages/search?q=egzamin+z", a.bearer)
		expectError(t, rec, http.StatusBadRequest, "invalid_input")
	})

	t.Run("a student outside the conversation gets 404", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		stranger := e.user(t)
		rec := e.api.Get("/chat/conversations/"+conversation.ID.String()+"/messages/search?q=hello", stranger.bearer)
		expectError(t, rec, http.StatusNotFound, "not_found")
	})

	t.Run("searching without a session returns 401", func(t *testing.T) {
		rec := e.api.Get("/chat/conversations/" + uuid.NewString() + "/messages/search?q=hello")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})
}

type aroundPage struct {
	Items       []Message
	NextCursor  *string
	NewerCursor *string
}

func TestListMessagesAround(t *testing.T) {
	e := newTestEnv(t)

	t.Run("a jump returns the message with older and newer ones, newest first", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		var sent []Message
		for range 9 {
			sent = append(sent, e.send(t, a, conversation.ID, "message"))
		}
		anchor := sent[4]

		page := decode[aroundPage](t, e.api.Get("/chat/conversations/"+conversation.ID.String()+"/messages?limit=4&aroundId="+anchor.ID.String(), a.bearer), http.StatusOK)
		if got := seqs(page.Items); !slices.Equal(got, []int64{7, 6, 5, 4}) {
			t.Errorf("page = %v, want [7 6 5 4]", got)
		}
		if page.NextCursor == nil || page.NewerCursor == nil {
			t.Fatalf("cursors = %v %v, want both", page.NextCursor, page.NewerCursor)
		}
	})

	t.Run("paging both ways from a jump covers every message exactly once", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		var sent []Message
		for range 11 {
			sent = append(sent, e.send(t, a, conversation.ID, "message"))
		}
		base := "/chat/conversations/" + conversation.ID.String() + "/messages?limit=3"

		page := decode[aroundPage](t, e.api.Get(base+"&aroundId="+sent[5].ID.String(), a.bearer), http.StatusOK)
		seen := seqs(page.Items)
		for next := page.NextCursor; next != nil; {
			p := decode[aroundPage](t, e.api.Get(base+"&cursor="+*next, a.bearer), http.StatusOK)
			seen = append(seen, seqs(p.Items)...)
			next = p.NextCursor
		}
		for newer := page.NewerCursor; newer != nil; {
			p := decode[aroundPage](t, e.api.Get(base+"&cursor="+*newer, a.bearer), http.StatusOK)
			if !slices.IsSortedFunc(p.Items, func(x, y Message) int { return int(y.Seq - x.Seq) }) {
				t.Errorf("newer page not newest first: %v", seqs(p.Items))
			}
			seen = append(seen, seqs(p.Items)...)
			newer = p.NewerCursor
		}

		slices.Sort(seen)
		want := make([]int64, 0, 11)
		for i := range 11 {
			want = append(want, int64(i+1))
		}
		if !slices.Equal(seen, want) {
			t.Errorf("seen = %v, want 1..11 once each", seen)
		}
	})

	t.Run("a jump to the newest message has no newer cursor", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		e.send(t, a, conversation.ID, "older")
		last := e.send(t, a, conversation.ID, "newest")

		page := decode[aroundPage](t, e.api.Get("/chat/conversations/"+conversation.ID.String()+"/messages?limit=1&aroundId="+last.ID.String(), a.bearer), http.StatusOK)
		if got := seqs(page.Items); !slices.Equal(got, []int64{last.Seq}) || page.NewerCursor != nil || page.NextCursor == nil {
			t.Errorf("page = %v newer %v next %v", got, page.NewerCursor, page.NextCursor)
		}
	})

	t.Run("aroundId with a cursor or changedAfter is rejected", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		e.send(t, a, conversation.ID, "older")
		m := e.send(t, a, conversation.ID, "hello")
		page := e.history(t, a, conversation.ID, "limit=1")
		base := "/chat/conversations/" + conversation.ID.String() + "/messages?aroundId=" + m.ID.String()

		expectError(t, e.api.Get(base+"&cursor="+*page.NextCursor, a.bearer), http.StatusBadRequest, "invalid_input")
		expectError(t, e.api.Get(base+"&changedAfter=0", a.bearer), http.StatusBadRequest, "invalid_input")
	})

	t.Run("a message of another conversation is not found", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		c := e.user(t)
		e.connect(t, a, c)
		other := e.direct(t, a, c)
		m := e.send(t, a, other.ID, "elsewhere")

		rec := e.api.Get("/chat/conversations/"+conversation.ID.String()+"/messages?aroundId="+m.ID.String(), a.bearer)
		expectError(t, rec, http.StatusNotFound, "not_found")
	})
}
