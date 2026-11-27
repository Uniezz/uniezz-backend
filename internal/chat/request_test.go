package chat

import (
	"net/http"
	"net/http/httptest"
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

func (e *testEnv) rawRequest(sender testUser, recipient testUser, key uuid.UUID, body string) *httptest.ResponseRecorder {
	return e.api.Post("/chat/requests", sender.bearer, keyHeader(key), map[string]any{"userId": recipient.id, "body": body})
}

func (e *testEnv) request(t *testing.T, sender, recipient testUser) Conversation {
	t.Helper()
	return decode[Conversation](t, e.rawRequest(sender, recipient, uuid.New(), "hello"), http.StatusCreated)
}

func (e *testEnv) conversationRows(t *testing.T, a, b testUser) int {
	t.Helper()
	var rows int
	err := e.pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM chat_conversations
		WHERE direct_user_low = LEAST($1::uuid, $2::uuid) AND direct_user_high = GREATEST($1::uuid, $2::uuid)`, a.id, b.id).Scan(&rows)
	if err != nil {
		t.Fatal(err)
	}
	return rows
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

	t.Run("opening a chat with a stranger creates nothing and asks for a request", func(t *testing.T) {
		sender, recipient := e.user(t), e.user(t)
		expectError(t, e.api.Post("/chat/conversations/direct", sender.bearer, map[string]any{"userId": recipient.id}), http.StatusConflict, "request_required")
		if rows := e.conversationRows(t, sender, recipient); rows != 0 {
			t.Errorf("conversations = %d, want 0", rows)
		}
	})

	t.Run("a request starts the conversation with its first message", func(t *testing.T) {
		sender, recipient := e.user(t), e.user(t)
		request := e.request(t, sender, recipient)
		if request.Origin == nil || *request.Origin != OriginRequest || *request.Status != StatusPending || *request.Incoming || request.LastSeq != 1 {
			t.Fatalf("request = %+v, want an outgoing pending request holding one message", request)
		}
		if !listed(e.requests(t, recipient), request.ID) {
			t.Errorf("request not listed for the recipient")
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
		if again := decode[Conversation](t, e.api.Post("/chat/conversations/direct", sender.bearer, map[string]any{"userId": recipient.id}), http.StatusOK); again.ID != request.ID {
			t.Errorf("reopening gave %s, want %s", again.ID, request.ID)
		}
	})

	t.Run("without a session it answers 401", func(t *testing.T) {
		recipient := e.user(t)
		rec := e.api.Post("/chat/requests", keyHeader(uuid.New()), map[string]any{"userId": recipient.id, "body": "hello"})
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("a request to a connection or into an existing conversation is refused", func(t *testing.T) {
		sender, recipient, friend := e.user(t), e.user(t), e.user(t)
		e.request(t, sender, recipient)
		expectError(t, e.rawRequest(sender, recipient, uuid.New(), "again"), http.StatusConflict, "conversation_exists")
		expectError(t, e.rawRequest(recipient, sender, uuid.New(), "hi"), http.StatusConflict, "conversation_exists")
		e.connect(t, sender, friend)
		expectError(t, e.rawRequest(sender, friend, uuid.New(), "hi"), http.StatusConflict, "conversation_exists")
		expectError(t, e.rawRequest(sender, sender, uuid.New(), "me"), http.StatusBadRequest, "invalid_input")
	})

	t.Run("the same key returns the stored request", func(t *testing.T) {
		sender, recipient := e.user(t), e.user(t)
		key := uuid.New()
		first := decode[Conversation](t, e.rawRequest(sender, recipient, key, "hello"), http.StatusCreated)
		rec := e.rawRequest(sender, recipient, key, "hello")
		again := decode[Conversation](t, rec, http.StatusCreated)
		if again.ID != first.ID || rec.Header().Get("Idempotent-Replayed") != "true" {
			t.Errorf("replay = %s %q, want %s and true", again.ID, rec.Header().Get("Idempotent-Replayed"), first.ID)
		}
		expectError(t, e.rawRequest(sender, recipient, key, "other"), http.StatusUnprocessableEntity, "idempotency_key_reused")
	})

	t.Run("a double tap stores one request with one message", func(t *testing.T) {
		sender, recipient := e.user(t), e.user(t)
		key := uuid.New()
		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for range 2 {
			wg.Go(func() {
				codes <- e.rawRequest(sender, recipient, key, "hello").Code
			})
		}
		wg.Wait()
		close(codes)
		for code := range codes {
			if code != http.StatusCreated {
				t.Errorf("status = %d, want 201", code)
			}
		}
		var conversations, messages int
		err := e.pool.QueryRow(t.Context(), `
			SELECT COUNT(DISTINCT c.id), COUNT(m.id)
			FROM chat_conversations c LEFT JOIN chat_messages m ON m.conversation_id = c.id
			WHERE c.direct_user_low = LEAST($1::uuid, $2::uuid) AND c.direct_user_high = GREATEST($1::uuid, $2::uuid)`, sender.id, recipient.id).Scan(&conversations, &messages)
		if err != nil {
			t.Fatal(err)
		}
		if conversations != 1 || messages != 1 {
			t.Errorf("conversations = %d messages = %d, want 1 and 1", conversations, messages)
		}
	})

	t.Run("the first message pushes a request, a reply pushes a message", func(t *testing.T) {
		sender, recipient := e.user(t), e.user(t)
		request := e.request(t, sender, recipient)
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

	t.Run("the sender gets one text message without links", func(t *testing.T) {
		sender, recipient := e.user(t), e.user(t)
		expectError(t, e.rawRequest(sender, recipient, uuid.New(), "see https://example.com"), http.StatusConflict, "request_pending")
		expectError(t, e.rawRequest(sender, recipient, uuid.New(), "or example.com"), http.StatusConflict, "request_pending")
		if rows := e.conversationRows(t, sender, recipient); rows != 0 {
			t.Errorf("a refused request left %d conversations", rows)
		}

		request := e.request(t, sender, recipient)
		expectError(t, e.rawSend(sender, request.ID, uuid.New(), map[string]any{"body": "again"}), http.StatusConflict, "request_pending")
	})

	t.Run("a reply from the recipient accepts the request", func(t *testing.T) {
		sender, recipient := e.user(t), e.user(t)
		request := e.request(t, sender, recipient)

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
		expectError(t, e.rawRequest(stranger, recipient, uuid.New(), "hello"), http.StatusNotFound, "not_found")
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
		expectError(t, e.rawRequest(elsewhere, recipient, uuid.New(), "hello"), http.StatusNotFound, "not_found")
	})

	t.Run("adults cannot write to minors without a match", func(t *testing.T) {
		adult, minor, unknown := e.user(t), e.user(t), e.user(t)
		e.setBirthDate(t, adult, time.Now().AddDate(-25, 0, 0))
		e.setBirthDate(t, minor, time.Now().AddDate(-17, 0, 0))

		expectError(t, e.rawRequest(adult, minor, uuid.New(), "hello"), http.StatusNotFound, "not_found")
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
		late := e.user(t)
		expectError(t, e.rawRequest(sender, late, uuid.New(), "hello"), http.StatusTooManyRequests, "rate_limited")
		if rows := e.conversationRows(t, sender, late); rows != 0 {
			t.Errorf("a limited request left %d conversations", rows)
		}
	})

	t.Run("a recipient holds at most 5 pending requests from new accounts", func(t *testing.T) {
		recipient := e.user(t)
		for range maxNewAccountRequests {
			e.request(t, e.user(t), recipient)
		}

		expectError(t, e.rawRequest(e.user(t), recipient, uuid.New(), "hello"), http.StatusTooManyRequests, "rate_limited")

		older := e.user(t)
		e.setCreatedAt(t, older, time.Now().Add(-3*24*time.Hour))
		e.request(t, older, recipient)
	})

	t.Run("account deletion removes requests and settings", func(t *testing.T) {
		sender, recipient := e.user(t), e.user(t)
		request := e.request(t, sender, recipient)
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
