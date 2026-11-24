// Package chat owns direct and group conversations, message requests between
// students without a Connection, messages with ordering and catch-up
// counters, read positions, and the live event stream. Access rules
// are in access.go and conversation_repository.go; PostgreSQL is the only
// source of truth, and the in-process hub only lowers latency.
package chat

import (
	"context"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/Uniezz/uniezz-backend/internal/push"
	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	sendLimit         = 30
	sendWindow        = time.Minute
	groupLimit        = 5
	groupWindow       = time.Hour
	requestLimit      = 20
	requestWindow     = 24 * time.Hour
	heartbeatInterval = 20 * time.Second
	writeDeadline     = 30 * time.Second
	closeGrace        = 2 * time.Second
	sessionCheckEvery = 5 * time.Minute
	publishTimeout    = 10 * time.Second
)

var _ mediaService = media.Module{}

type mediaService interface {
	CheckUpload(ctx context.Context, mediaID, ownerUserID uuid.UUID, kind media.Kind) error
	MarkReady(ctx context.Context, mediaID, ownerUserID uuid.UUID, kind media.Kind) error
	MarkDeletingMany(ctx context.Context, mediaIDs []uuid.UUID) error
	Delete(ctx context.Context, mediaID uuid.UUID) error
	URLOf(ctx context.Context, ref media.Ref) (string, error)
}

var _ notifier = push.Module{}

type notifier interface {
	Notify(ctx context.Context, args push.SendArgs) error
	Wake()
}

var _ limiter = (*ratelimit.Limiter)(nil)

type limiter interface {
	Allow(ctx context.Context, key string) error
}

type Module struct {
	tx            database.Transactor
	conversations *conversationRepository
	members       *memberRepository
	messages      *messageRepository
	students      *studentRepository
	requests      *requestRepository
	settings      *settingsRepository
	sessions      *sessionRepository
	media         mediaService
	push          notifier
	sends         limiter
	groups        limiter
	requestLimit  limiter
	hub           *hub
	heartbeat     time.Duration
	sessionCheck  time.Duration
	closeWait     time.Duration
	now           func() time.Time
}

func New(pool *pgxpool.Pool, mediaModule mediaService, pushModule notifier) Module {
	db := database.New(pool)
	return Module{
		tx:            db,
		conversations: newConversationRepository(db),
		members:       newMemberRepository(db),
		messages:      newMessageRepository(db),
		students:      newStudentRepository(db),
		requests:      newRequestRepository(db),
		settings:      newSettingsRepository(db),
		sessions:      newSessionRepository(db),
		media:         mediaModule,
		push:          pushModule,
		sends:         ratelimit.New(pool, "chat-send", sendLimit, sendWindow),
		groups:        ratelimit.New(pool, "chat-group", groupLimit, groupWindow),
		requestLimit:  ratelimit.New(pool, "chat-request", requestLimit, requestWindow),
		hub:           newHub(),
		heartbeat:     heartbeatInterval,
		sessionCheck:  sessionCheckEvery,
		closeWait:     closeGrace,
		now:           time.Now,
	}
}

func (m Module) Register(api huma.API) {
	conversations := &conversationHandler{chat: m}
	members := &memberHandler{chat: m}
	messages := &messageHandler{chat: m}
	stream := &streamHandler{chat: m}
	requests := &requestHandler{chat: m}
	settings := &settingsHandler{chat: m}

	huma.Register(api, openDirectConversationOperation, conversations.openDirectConversation)
	huma.Register(api, createGroupConversationOperation, conversations.createGroupConversation)
	huma.Register(api, listChatConversationsOperation, conversations.listConversations)
	huma.Register(api, getChatConversationOperation, conversations.getConversation)
	huma.Register(api, updateChatConversationOperation, conversations.updateConversation)
	huma.Register(api, listChatMembersOperation, members.listMembers)
	huma.Register(api, addChatMembersOperation, members.addMembers)
	huma.Register(api, leaveChatConversationOperation, members.leaveConversation)
	huma.Register(api, removeChatMemberOperation, members.removeMember)
	huma.Register(api, markChatReadOperation, members.markRead)
	huma.Register(api, getChatUnreadOperation, members.getUnread)
	huma.Register(api, listChatMessagesOperation, messages.listMessages)
	huma.Register(api, sendChatMessageOperation, messages.sendMessage)
	huma.Register(api, getChatMessageOperation, messages.getMessage)
	huma.Register(api, deleteChatMessageOperation, messages.deleteMessage)
	huma.Register(api, openChatStreamOperation, stream.openStream)
	huma.Register(api, listChatRequestsOperation, requests.listRequests)
	huma.Register(api, createChatRequestOperation, requests.createRequest)
	huma.Register(api, acceptChatRequestOperation, requests.acceptRequest)
	huma.Register(api, declineChatRequestOperation, requests.declineRequest)
	huma.Register(api, getMyChatSettingsOperation, settings.getSettings)
	huma.Register(api, updateMyChatSettingsOperation, settings.updateSettings)
}

func (m Module) Drain() {
	m.hub.drain(reconnectFrame)
}
