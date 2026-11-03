package chat

import (
	"context"
	"unicode/utf8"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxMessageLength = 4000

type Module struct {
	repo *repository
}

func New(pool *pgxpool.Pool) Module {
	return Module{repo: newRepository(database.New(pool))}
}

func (m Module) getOrCreateThread(ctx context.Context, userID, otherID uuid.UUID) (*Thread, error) {
	return m.repo.getOrCreateThread(ctx, userID, otherID)
}

func (m Module) listMessages(ctx context.Context, threadID, userID uuid.UUID) ([]Message, error) {
	return m.repo.listMessages(ctx, threadID, userID)
}

func (m Module) createMessage(ctx context.Context, threadID, senderID uuid.UUID, raw string) (*Message, error) {
	content := text.Plain(raw, true)
	if content == "" || utf8.RuneCountInString(content) > maxMessageLength {
		return nil, ErrInvalidInput
	}
	return m.repo.createMessage(ctx, threadID, senderID, content)
}
