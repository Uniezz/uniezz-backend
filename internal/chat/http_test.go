package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestChatHTTPAuthorizationAndMessages(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	t.Cleanup(pool.Close)

	first := createChatHTTPUser(t, ctx, pool)
	second := createChatHTTPUser(t, ctx, pool)
	third := createChatHTTPUser(t, ctx, pool)

	authModule, err := auth.New(pool, &config.Config{UsosConfigs: map[string]config.UsosCredentials{
		"umcs":  {BaseURL: "https://usos.test", ConsumerKey: "key", ConsumerSecret: "secret"},
		"umlub": {BaseURL: "https://usos.test", ConsumerKey: "key", ConsumerSecret: "secret"},
	}}, nil, nil)
	if err != nil {
		t.Fatalf("initialize auth module: %v", err)
	}

	_, api := humatest.New(t, httpapi.NewConfig(&config.Config{}))
	api.UseMiddleware(authModule.Guard(api))
	New(pool).Register(api)

	unauthorized := api.Post("/chats", map[string]string{"otherUserId": second.id.String()})
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request: got %d, want 401: %s", unauthorized.Code, unauthorized.Body.String())
	}

	created := decodeChatHTTPResponse[struct {
		ID uuid.UUID `json:"id"`
	}](t, api.Post("/chats", first.bearer, map[string]string{
		"otherUserId": second.id.String(),
	}), http.StatusOK)
	if created.ID == uuid.Nil {
		t.Fatal("start chat returned an empty thread ID")
	}

	sent := decodeChatHTTPResponse[struct {
		ID      uuid.UUID `json:"id"`
		Content string    `json:"content"`
	}](t, api.Post("/chats/"+created.ID.String()+"/messages", first.bearer, map[string]string{
		"content": "Hello from the first student",
	}), http.StatusCreated)
	if sent.Content != "Hello from the first student" {
		t.Fatalf("sent message content = %q", sent.Content)
	}

	type message struct {
		ID      uuid.UUID `json:"id"`
		Content string    `json:"content"`
	}
	history := decodeChatHTTPResponse[struct {
		Items []message `json:"items"`
	}](t, api.Get("/chats/"+created.ID.String()+"/messages", second.bearer), http.StatusOK)
	if len(history.Items) != 1 || history.Items[0].ID != sent.ID {
		t.Fatalf("unexpected chat history: %+v", history.Items)
	}

	for _, rec := range []*httptest.ResponseRecorder{
		api.Get("/chats/"+created.ID.String()+"/messages", third.bearer),
		api.Post("/chats/"+created.ID.String()+"/messages", third.bearer, map[string]string{
			"content": "I am not a participant",
		}),
	} {
		if rec.Code != http.StatusNotFound {
			t.Errorf("third user request: got %d, want 404: %s", rec.Code, rec.Body.String())
		}
	}
}

type chatHTTPUser struct {
	id     uuid.UUID
	bearer string
}

func createChatHTTPUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool) chatHTTPUser {
	t.Helper()

	var id uuid.UUID
	err := pool.QueryRow(ctx, `
        INSERT INTO users (university_id, email)
        VALUES ('kul', $1)
        RETURNING id`, "chat-http-"+uuid.NewString()+"@example.test").Scan(&id)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id)
	})

	token := auth.GenerateSessionToken()
	hash := token.Hash()
	if _, err := auth.NewSessionRepository(database.New(pool)).CreateSession(
		ctx, id, hash[:], time.Now().Add(time.Hour),
	); err != nil {
		t.Fatalf("create test session: %v", err)
	}

	return chatHTTPUser{id: id, bearer: "Authorization: Bearer " + token.String()}
}

func decodeChatHTTPResponse[T any](t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) T {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("got status %d, want %d: %s", rec.Code, wantStatus, rec.Body.String())
	}
	var body T
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v: %s", err, rec.Body.String())
	}
	return body
}
