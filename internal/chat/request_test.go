package chat

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/push"
	"github.com/google/uuid"
)

type conversationPage struct {
	Items      []Conversation
	NextCursor *string
}

func (e *testEnv) request(t *testing.T, sender, recipient testUser) Conversation {
	t.Helper()
	return decode[Conversation](t, e.api.Post("/chat/conversations/direct", sender.bearer, map[string]any{"userId": recipient.id}), http.StatusCreated)
}

func (e *testEnv) setBirthDate(t *testing.T, u testUser, birthDate time.Time) {
	t.Helper()
	if _, err := e.pool.Exec(t.Context(), "INSERT INTO connection_profiles (user_id, birth_date) VALUES ($1, $2)", u.id, birthDate); err != nil {
		t.Fatalf("set birth date: %v", err)
	}
}

func (e *testEnv) setCreatedAt(t *testing.T, u testUser, createdAt time.Time) {
	t.Helper()
	if _, err := e.pool.Exec(t.Context(), "UPDATE users SET created_at = $2 WHERE id = $1", u.id, createdAt); err != nil {
		t.Fatalf("set created_at: %v", err)
	}
}

func listed(page conversationPage, id uuid.UUID) bool {
	for _, c := range page.Items {
		if c.ID == id {
			return true
		}
	}
	return false
}

func (e *testEnv) requests(t *testing.T, u testUser) conversationPage {
	t.Helper()
	return decode[conversationPage](t, e.api.Get("/chat/requests?limit=50", u.bearer), http.StatusOK)
}

func (e *testEnv) inbox(t *testing.T, u testUser) conversationPage {
	t.Helper()
	return decode[conversationPage](t, e.api.Get("/chat/conversations?limit=50", u.bearer), http.StatusOK)
}

func TestMessageRequests(t *testing.T) {
	e := newTestEnv(t)

	t.Run("a stranger's request reaches the recipient only once it holds a message", func(t *testing.T) {
		sender, recipient := e.user(t), e.user(t)
		request := e.request(t, sender, recipient)
		if request.Origin == nil || *request.Origin != OriginRequest || *request.Status != StatusPending || *request.Incoming {
			t.Fatalf("request = %+v, want an outgoing pending request", request)
		}
		if listed(e.requests(t, recipient), request.ID) {
			t.Errorf("empty request listed for the recipient")
		}

		e.send(t, sender, request.ID, "hi, we share a class")
		if !listed(e.requests(t, recipient), request.ID) {
			t.Errorf("request with a message not listed for the recipient")
		}
		if listed(e.inbox(t, recipient), request.ID) {
			t.Errorf("pending request listed in the recipient's conversations")
		}
		if !listed(e.inbox(t, sender), request.ID) {
			t.Errorf("pending request missing from the sender's conversations")
		}
		incoming := e.conversation(t, recipient, request.ID)
		if !*incoming.Incoming || *incoming.Status != StatusPending {
			t.Errorf("recipient sees %+v, want an incoming pending request", incoming)
		}
		if page := e.history(t, recipient, request.ID, ""); len(page.Items) != 1 {
			t.Errorf("recipient reads %d messages, want 1", len(page.Items))
		}
	})

	t.Run("the first message pushes a request, a reply pushes a message", func(t *testing.T) {
		sender, recipient := e.user(t), e.user(t)
		request := e.request(t, sender, recipient)
		e.send(t, sender, request.ID, "hello")
		e.send(t, recipient, request.ID, "hi back")

		var events []push.Event
		for _, n := range e.push.notices() {
			if n.ConversationID != nil && *n.ConversationID == request.ID {
				events = append(events, n.Event)
				if len(n.RecipientIDs) != 1 || n.Preview == "" {
					t.Errorf("notice = %+v", n)
				}
			}
		}
		if len(events) != 2 || events[0] != push.EventRequest || events[1] != push.EventMessage {
			t.Errorf("events = %v, want request then message", events)
		}
	})

	t.Run("the sender gets one text message without links or photos", func(t *testing.T) {
		sender, recipient := e.user(t), e.user(t)
		request := e.request(t, sender, recipient)

		expectError(t, e.rawSend(sender, request.ID, uuid.New(), map[string]any{"body": "see https://example.com"}), http.StatusConflict, "request_pending")
		expectError(t, e.rawSend(sender, request.ID, uuid.New(), map[string]any{"body": "or example.com"}), http.StatusConflict, "request_pending")
		image := e.pendingImage(t, sender)
		expectError(t, e.rawSend(sender, request.ID, uuid.New(), map[string]any{"attachmentIds": []uuid.UUID{image}}), http.StatusConflict, "request_pending")

		e.send(t, sender, request.ID, "hello")
		expectError(t, e.rawSend(sender, request.ID, uuid.New(), map[string]any{"body": "again"}), http.StatusConflict, "request_pending")
	})

	t.Run("a double tap on the first message stores one message", func(t *testing.T) {
		sender, recipient := e.user(t), e.user(t)
		request := e.request(t, sender, recipient)

		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for range 2 {
			wg.Go(func() {
				codes <- e.rawSend(sender, request.ID, uuid.New(), map[string]any{"body": "hello"}).Code
			})
		}
		wg.Wait()
		close(codes)
		created := 0
		for code := range codes {
			if code == http.StatusCreated {
				created++
			} else if code != http.StatusConflict {
				t.Errorf("status = %d, want 201 or 409", code)
			}
		}
		var rows int
		if err := e.pool.QueryRow(t.Context(), "SELECT COUNT(*) FROM chat_messages WHERE conversation_id = $1", request.ID).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if created != 1 || rows != 1 {
			t.Errorf("created = %d rows = %d, want 1 and 1", created, rows)
		}
	})

	t.Run("a reply from the recipient accepts the request", func(t *testing.T) {
		sender, recipient := e.user(t), e.user(t)
		request := e.request(t, sender, recipient)
		e.send(t, sender, request.ID, "hello")

		e.send(t, recipient, request.ID, "hi back")
		if c := e.conversation(t, sender, request.ID); *c.Status != StatusApproved {
			t.Errorf("status after reply = %s, want approved", *c.Status)
		}
		e.send(t, sender, request.ID, "great, and a link: example.com")
		if !listed(e.inbox(t, recipient), request.ID) {
			t.Errorf("accepted request missing from the recipient's conversations")
		}
	})

	t.Run("accepting turns the request into a conversation, and only the recipient can", func(t *testing.T) {
		sender, recipient := e.user(t), e.user(t)
		request := e.request(t, sender, recipient)
		path := "/chat/requests/" + request.ID.String() + "/acceptance"
		expectError(t, e.api.Put(path, recipient.bearer, map[string]any{}), http.StatusNotFound, "not_found")

		e.send(t, sender, request.ID, "hello")
		expectError(t, e.api.Put(path, sender.bearer, map[string]any{}), http.StatusNotFound, "not_found")
		accepted := decode[Conversation](t, e.api.Put(path, recipient.bearer, map[string]any{}), http.StatusOK)
		if *accepted.Status != StatusApproved || *accepted.Origin != OriginRequest {
			t.Errorf("accepted = %+v", accepted)
		}
		decode[Conversation](t, e.api.Put(path, recipient.bearer, map[string]any{}), http.StatusOK)
		if listed(e.requests(t, recipient), request.ID) {
			t.Errorf("accepted request still listed as a request")
		}
	})

	t.Run("declining hides the request from the recipient and not from the sender", func(t *testing.T) {
		sender, recipient := e.user(t), e.user(t)
		request := e.request(t, sender, recipient)
		e.send(t, sender, request.ID, "hello")
		path := "/chat/requests/" + request.ID.String()

		expectError(t, e.api.Delete(path, sender.bearer), http.StatusNotFound, "not_found")
		if rec := e.api.Delete(path, recipient.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("decline: %d %s", rec.Code, rec.Body.String())
		}
		if rec := e.api.Delete(path, recipient.bearer); rec.Code != http.StatusNoContent {
			t.Errorf("declining again: %d, want 204", rec.Code)
		}
		if listed(e.requests(t, recipient), request.ID) {
			t.Errorf("declined request still listed")
		}
		expectError(t, e.api.Get("/chat/conversations/"+request.ID.String(), recipient.bearer), http.StatusNotFound, "not_found")
		expectError(t, e.rawSend(recipient, request.ID, uuid.New(), map[string]any{"body": "hm"}), http.StatusNotFound, "not_found")
		if c := e.conversation(t, sender, request.ID); *c.Status != StatusPending {
			t.Errorf("sender sees %s, want pending", *c.Status)
		}
		if again := decode[Conversation](t, e.api.Post("/chat/conversations/direct", sender.bearer, map[string]any{"userId": recipient.id}), http.StatusOK); *again.Status != StatusPending {
			t.Errorf("sender reopening sees %s, want pending", *again.Status)
		}
	})

	t.Run("a match after a request approves it and keeps its origin", func(t *testing.T) {
		sender, recipient := e.user(t), e.user(t)
		request := e.request(t, sender, recipient)
		e.send(t, sender, request.ID, "hello")
		e.connect(t, sender, recipient)

		opened := decode[Conversation](t, e.api.Post("/chat/conversations/direct", sender.bearer, map[string]any{"userId": recipient.id}), http.StatusOK)
		if opened.ID != request.ID || *opened.Status != StatusApproved || *opened.Origin != OriginRequest {
			t.Errorf("opened = %+v, want the approved request", opened)
		}
		e.send(t, sender, request.ID, "now we matched")
	})

	t.Run("who can message me limits new requests", func(t *testing.T) {
		recipient := e.user(t)
		settings := decode[Settings](t, e.api.Get("/chat/settings/me", recipient.bearer), http.StatusOK)
		if settings.WhoCanMessage != WhoEveryone {
			t.Errorf("default = %s, want everyone", settings.WhoCanMessage)
		}

		decode[Settings](t, e.api.Patch("/chat/settings/me", recipient.bearer, map[string]any{"whoCanMessage": WhoMatches}), http.StatusOK)
		stranger := e.user(t)
		expectError(t, e.api.Post("/chat/conversations/direct", stranger.bearer, map[string]any{"userId": recipient.id}), http.StatusNotFound, "not_found")
		friend := e.user(t)
		e.connect(t, friend, recipient)
		e.direct(t, friend, recipient)

		updated := decode[Settings](t, e.api.Patch("/chat/settings/me", recipient.bearer, map[string]any{"whoCanMessage": WhoUniversity}), http.StatusOK)
		if updated.WhoCanMessage != WhoUniversity {
			t.Errorf("updated = %s, want university", updated.WhoCanMessage)
		}
		e.request(t, stranger, recipient)
		elsewhere := e.user(t)
		if _, err := e.pool.Exec(t.Context(), "UPDATE users SET university_id = 'kul' WHERE id = $1", elsewhere.id); err != nil {
			t.Fatal(err)
		}
		expectError(t, e.api.Post("/chat/conversations/direct", elsewhere.bearer, map[string]any{"userId": recipient.id}), http.StatusNotFound, "not_found")
	})

	t.Run("adults cannot write to minors without a match", func(t *testing.T) {
		adult, minor, unknown := e.user(t), e.user(t), e.user(t)
		e.setBirthDate(t, adult, time.Now().AddDate(-25, 0, 0))
		e.setBirthDate(t, minor, time.Now().AddDate(-17, 0, 0))

		expectError(t, e.api.Post("/chat/conversations/direct", adult.bearer, map[string]any{"userId": minor.id}), http.StatusNotFound, "not_found")
		e.request(t, minor, adult)
		e.request(t, adult, unknown)

		e.connect(t, adult, minor)
		e.direct(t, adult, minor)
	})

	t.Run("a sender starts at most 20 requests a day", func(t *testing.T) {
		sender := e.user(t)
		for range requestLimit {
			e.request(t, sender, e.user(t))
		}
		expectError(t, e.api.Post("/chat/conversations/direct", sender.bearer, map[string]any{"userId": e.user(t).id}), http.StatusTooManyRequests, "rate_limited")
	})

	t.Run("a recipient holds at most 5 pending requests from new accounts", func(t *testing.T) {
		recipient := e.user(t)
		for range maxNewAccountRequests {
			sender := e.user(t)
			e.send(t, sender, e.request(t, sender, recipient).ID, "hello")
		}

		late := e.user(t)
		lateRequest := e.request(t, late, recipient)
		expectError(t, e.rawSend(late, lateRequest.ID, uuid.New(), map[string]any{"body": "hello"}), http.StatusTooManyRequests, "rate_limited")

		older := e.user(t)
		e.setCreatedAt(t, older, time.Now().Add(-3*24*time.Hour))
		e.send(t, older, e.request(t, older, recipient).ID, "hello")
	})

	t.Run("account deletion removes requests and settings", func(t *testing.T) {
		sender, recipient := e.user(t), e.user(t)
		request := e.request(t, sender, recipient)
		e.send(t, sender, request.ID, "hello")
		decode[Settings](t, e.api.Patch("/chat/settings/me", sender.bearer, map[string]any{"whoCanMessage": WhoMatches}), http.StatusOK)

		if err := e.module.ForgetUser(t.Context(), sender.id); err != nil {
			t.Fatalf("ForgetUser failed: %v", err)
		}
		var conversations, settings int
		err := e.pool.QueryRow(t.Context(), `
			SELECT (SELECT COUNT(*) FROM chat_conversations WHERE id = $1),
			       (SELECT COUNT(*) FROM chat_settings WHERE user_id = $2)`, request.ID, sender.id).Scan(&conversations, &settings)
		if err != nil {
			t.Fatal(err)
		}
		if conversations != 0 || settings != 0 {
			t.Errorf("conversations = %d settings = %d, want 0 and 0", conversations, settings)
		}
	})
}
