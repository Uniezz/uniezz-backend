package chat

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type handler struct {
	chat Module
}

type threadResponse struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	OtherUserID uuid.UUID `json:"otherUserId" format:"uuid"`
}

type messageResponse struct {
	ID        uuid.UUID `json:"id" format:"uuid"`
	ThreadID  uuid.UUID `json:"threadId" format:"uuid"`
	SenderID  uuid.UUID `json:"senderId" format:"uuid"`
	Content   string    `json:"content"`
	CreatedAt string    `json:"createdAt" format:"date-time"`
}

type startChatInput struct {
	Body struct {
		OtherUserID uuid.UUID `json:"otherUserId" format:"uuid"`
	}
}

type startChatOutput struct {
	Body threadResponse
}

var startChatOperation = auth.Protected(huma.Operation{
	OperationID: "startChat",
	Method:      http.MethodPost,
	Path:        "/chats",
	Summary:     "Start a one-to-one chat or return the existing thread",
	Tags:        []string{"chat"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *handler) startChat(ctx context.Context, in *startChatInput) (*startChatOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Error(http.StatusUnauthorized, "unauthorized", nil)
	}

	thread, err := h.chat.getOrCreateThread(ctx, userID, in.Body.OtherUserID)
	if err != nil {
		return nil, apiError(err)
	}

	otherID := thread.UserLowID
	if otherID == userID {
		otherID = thread.UserHighID
	}
	return &startChatOutput{Body: threadResponse{
		ID:          thread.ID,
		OtherUserID: otherID,
	}}, nil
}

type listMessagesInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

type listMessagesOutput struct {
	Body struct {
		Items []messageResponse `json:"items"`
	}
}

var listMessagesOperation = auth.Protected(huma.Operation{
	OperationID: "listChatMessages",
	Method:      http.MethodGet,
	Path:        "/chats/{id}/messages",
	Summary:     "List messages in a chat thread",
	Description: "Messages are returned oldest first. Only thread participants may read them.",
	Tags:        []string{"chat"},
	Errors:      []int{http.StatusNotFound},
})

func (h *handler) listMessages(ctx context.Context, in *listMessagesInput) (*listMessagesOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Error(http.StatusUnauthorized, "unauthorized", nil)
	}

	messages, err := h.chat.listMessages(ctx, in.ID, userID)
	if err != nil {
		return nil, apiError(err)
	}

	out := &listMessagesOutput{}
	out.Body.Items = make([]messageResponse, 0, len(messages))
	for _, message := range messages {
		out.Body.Items = append(out.Body.Items, messageResponse{
			ID:        message.ID,
			ThreadID:  message.ThreadID,
			SenderID:  message.SenderID,
			Content:   message.Content,
			CreatedAt: message.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
		})
	}
	return out, nil
}

type createMessageInput struct {
	ID   uuid.UUID `path:"id" format:"uuid"`
	Body struct {
		Content string `json:"content" minLength:"1" maxLength:"4000"`
	}
}

type createMessageOutput struct {
	Body messageResponse
}

var createMessageOperation = auth.Protected(huma.Operation{
	OperationID:   "createChatMessage",
	Method:        http.MethodPost,
	Path:          "/chats/{id}/messages",
	Summary:       "Send a message to a chat thread",
	Description:   "Only thread participants may send messages.",
	Tags:          []string{"chat"},
	DefaultStatus: http.StatusCreated,
	Errors:        []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *handler) createMessage(ctx context.Context, in *createMessageInput) (*createMessageOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Error(http.StatusUnauthorized, "unauthorized", nil)
	}

	message, err := h.chat.createMessage(ctx, in.ID, userID, in.Body.Content)
	if err != nil {
		return nil, apiError(err)
	}
	return &createMessageOutput{Body: messageResponse{
		ID:        message.ID,
		ThreadID:  message.ThreadID,
		SenderID:  message.SenderID,
		Content:   message.Content,
		CreatedAt: message.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
	}}, nil
}

func (m Module) Register(api huma.API) {
	h := &handler{chat: m}
	huma.Register(api, startChatOperation, h.startChat)
	huma.Register(api, listMessagesOperation, h.listMessages)
	huma.Register(api, createMessageOperation, h.createMessage)
}
