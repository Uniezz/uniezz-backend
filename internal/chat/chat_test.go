package chat

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOneToOneChat(t *testing.T) {
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

	first := createChatTestUser(t, ctx, pool)
	second := createChatTestUser(t, ctx, pool)
	third := createChatTestUser(t, ctx, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			"DELETE FROM users WHERE id = ANY($1)", []uuid.UUID{first, second, third})
	})

	module := New(pool)

	if _, err := module.getOrCreateThread(ctx, first, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("starting chat with unknown user should return ErrNotFound; got %v", err)
	}

	thread, err := module.getOrCreateThread(ctx, first, second)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	sameThread, err := module.getOrCreateThread(ctx, second, first)
	if err != nil {
		t.Fatalf("get existing thread: %v", err)
	}
	if sameThread.ID != thread.ID {
		t.Fatalf("participants got different threads: %s and %s", thread.ID, sameThread.ID)
	}

	firstMessage, err := module.createMessage(ctx, thread.ID, first, "first message")
	if err != nil {
		t.Fatalf("create first message: %v", err)
	}
	secondMessage, err := module.createMessage(ctx, thread.ID, second, "second message")
	if err != nil {
		t.Fatalf("create second message: %v", err)
	}

	// Fix timestamps so the ordering assertion is deterministic.
	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, message := range []*Message{firstMessage, secondMessage} {
		_, err := pool.Exec(ctx,
			"UPDATE chat_messages SET created_at = $2 WHERE id = $1",
			message.ID, baseTime.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatalf("set message timestamp: %v", err)
		}
	}

	messages, err := module.listMessages(ctx, thread.ID, first)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 || messages[0].ID != firstMessage.ID || messages[1].ID != secondMessage.ID {
		t.Fatalf("messages are not in send order: %+v", messages)
	}

	if _, err := module.listMessages(ctx, thread.ID, third); !errors.Is(err, ErrNotFound) {
		t.Errorf("third user should not read thread; got error %v", err)
	}
	if _, err := module.createMessage(ctx, thread.ID, third, "intruder"); !errors.Is(err, ErrNotFound) {
		t.Errorf("third user should not send to thread; got error %v", err)
	}
}

func createChatTestUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()

	email := fmt.Sprintf("chat-%s@example.test", uuid.NewString())
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
        INSERT INTO users (university_id, email)
        VALUES ('kul', $1)
        RETURNING id`, email).Scan(&id)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return id
}
