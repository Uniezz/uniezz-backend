package chat

import (
	"context"
	"net/http"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type messageHandler struct {
	chat Module
}

type MessageKind string

const (
	MessageText     MessageKind = "text"
	MessageSystem   MessageKind = "system"
	MessagePoll     MessageKind = "poll"
	MessageEvent    MessageKind = "event"
	MessageLocation MessageKind = "location"
	MessagePost     MessageKind = "post"
)

func (MessageKind) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	schema.Enum = nil
	if schema.Extensions == nil {
		schema.Extensions = map[string]any{}
	}
	schema.Extensions["x-extensible-enum"] = []any{MessageText, MessageSystem, MessagePoll, MessageEvent, MessageLocation, MessagePost}
	return schema
}

type Attachment struct {
	MediaID   uuid.UUID `json:"mediaId" format:"uuid"`
	URL       string    `json:"url" doc:"Presigned download URL, valid until expiresAt; fetch the message again for a fresh one"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type LinkPreview struct {
	URL         string  `json:"url" example:"https://www.umcs.pl/en/" doc:"The previewed link as found in the body"`
	Title       *string `json:"title" nullable:"true" example:"Maria Curie-Skłodowska University" doc:"Null when the page has none"`
	Description *string `json:"description" nullable:"true" doc:"Null when the page has none"`
	SiteName    *string `json:"siteName" nullable:"true" example:"UMCS" doc:"Null when the page has none"`
	ImageURL    *string `json:"imageUrl" nullable:"true" doc:"A copy stored by Uniezz, presigned like attachments; null when the page has no usable image"`
}

type Message struct {
	ID             uuid.UUID      `json:"id" format:"uuid"`
	ConversationID uuid.UUID      `json:"conversationId" format:"uuid"`
	Seq            int64          `json:"seq" doc:"Position in the conversation; display order"`
	ChangeSeq      int64          `json:"changeSeq" doc:"Sync cursor value of the message's latest change"`
	Kind           MessageKind    `json:"kind" doc:"More kinds may be added; show an unknown kind as its body text"`
	Sender         Student        `json:"sender"`
	Body           *string        `json:"body" nullable:"true" doc:"Null for system messages, image-only messages and deleted messages"`
	Payload        map[string]any `json:"payload" doc:"System messages: {event, actorId, params}; empty for others. Clients translate the event code"`
	ClientID       *uuid.UUID     `json:"clientId" format:"uuid" nullable:"true" doc:"The Idempotency-Key the sender used; null for system messages"`
	Attachments    []Attachment   `json:"attachments"`
	Poll           *Poll          `json:"poll,omitempty" doc:"Set when kind is poll and the message is not deleted"`
	LinkPreview    *LinkPreview   `json:"linkPreview,omitempty" doc:"Preview of the first https link in the body. Omitted while it is being fetched (a message.updated event follows), when fetching failed, and when the message has no link"`
	CreatedAt      time.Time      `json:"createdAt"`
	DeletedAt      *time.Time     `json:"deletedAt" nullable:"true" doc:"Set when the sender deleted the message; body, payload and attachments are then empty"`
}

type listChatMessagesInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
	httpapi.PageQuery
	ChangedAfter string `query:"changedAfter" pattern:"^[0-9]{1,18}$" maxLength:"18" doc:"Catch-up mode: list messages created or changed after this changeSeq, oldest change first. Pass nextCursor as cursor to continue. Without it, messages are listed newest first by seq"`
	AroundID     string `query:"aroundId" format:"uuid" doc:"Jump mode: a page around this message (it and older ones, then newer ones), newest first. Not allowed with cursor or changedAfter. Pass nextCursor for older pages and newerCursor for newer pages"`
}

type listChatMessagesOutput struct {
	Body struct {
		Items       []Message `json:"items"`
		NextCursor  *string   `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get older messages (or the next catch-up page); null when there are none"`
		NewerCursor *string   `json:"newerCursor" nullable:"true" doc:"Pass as cursor to get newer messages after a jump (aroundId); null when there are none or outside jump mode. Every page is still newest first"`
	}
}

var listChatMessagesOperation = auth.Authenticated(huma.Operation{
	OperationID: "listChatMessages",
	Method:      http.MethodGet,
	Path:        "/chat/conversations/{id}/messages",
	Summary:     "List messages of a conversation, or the ones that changed",
	Description: "History mode lists newest first by seq; aroundId jumps to a message, for example a search result. Catch-up mode (changedAfter) lists what was created or changed after a changeSeq, oldest change first, including deleted messages. Messages from students blocked either way or banned are left out.",
	Tags:        []string{"chat"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *messageHandler) listMessages(ctx context.Context, in *listChatMessagesInput) (*listChatMessagesOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	items, next, newer, err := h.chat.listMessages(ctx, messageQuery{
		ViewerID:       viewerID,
		ConversationID: in.ID,
		Cursor:         in.Cursor,
		ChangedAfter:   in.ChangedAfter,
		AroundID:       in.AroundID,
		Limit:          in.Limit,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}

	out := &listChatMessagesOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	out.Body.NewerCursor = newer
	return out, nil
}

type searchChatMessagesInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
	Q  string    `query:"q" required:"true" minLength:"3" maxLength:"200" example:"exam room" doc:"1 to 5 words of at least 3 characters each; every word must appear in the message, case and accents ignored"`
	httpapi.PageQuery
}

type searchChatMessagesOutput struct {
	Body struct {
		Items      []Message `json:"items"`
		NextCursor *string   `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get older matches; null on the last page"`
	}
}

var searchChatMessagesOperation = auth.Authenticated(huma.Operation{
	OperationID: "searchChatMessages",
	Method:      http.MethodGet,
	Path:        "/chat/conversations/{id}/messages/search",
	Summary:     "Search the text of messages in a conversation",
	Description: "Matches newest first by seq, without ranking; open a match with listChatMessages and aroundId. Deleted messages and messages from students blocked either way or banned are never returned.",
	Tags:        []string{"chat"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *messageHandler) searchMessages(ctx context.Context, in *searchChatMessagesInput) (*searchChatMessagesOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	items, next, err := h.chat.searchMessages(ctx, searchQuery{
		ViewerID:       viewerID,
		ConversationID: in.ID,
		Q:              in.Q,
		Cursor:         in.Cursor,
		Limit:          in.Limit,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}

	out := &searchChatMessagesOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

type sendChatMessageInput struct {
	ID             uuid.UUID `path:"id" format:"uuid"`
	IdempotencyKey string    `header:"Idempotency-Key" required:"true" minLength:"36" maxLength:"36" doc:"A UUID, stored as the message's clientId. Repeating the request with the same key and body returns the stored message"`
	Body           struct {
		Body          string       `json:"body,omitempty" maxLength:"16000" doc:"Up to 4000 characters after HTML is stripped; may be empty only with attachments"`
		AttachmentIDs []uuid.UUID  `json:"attachmentIds,omitempty" maxItems:"5" uniqueItems:"true" doc:"Uploads of kind chat, shown in this order; images only"`
		NoPreview     bool         `json:"noPreview,omitempty" doc:"Do not preview the first link of the body"`
		Poll          *pollRequest `json:"poll,omitempty" doc:"Send a poll instead of text; body and attachmentIds must then be empty"`
	}
}

type sendChatMessageOutput struct {
	Replayed string `header:"Idempotent-Replayed" doc:"true when the message was stored by an earlier request with the same key"`
	Body     Message
}

var sendChatMessageOperation = auth.Authenticated(huma.Operation{
	OperationID:   "sendChatMessage",
	Method:        http.MethodPost,
	Path:          "/chat/conversations/{id}/messages",
	Summary:       "Send a text message with up to 5 images, or a poll",
	Description:   "Same key and body: 201 with the stored message. Same key, other body: 422. Same key after the message was deleted: 410. At most 30 messages per minute.",
	Tags:          []string{"chat"},
	DefaultStatus: http.StatusCreated,
	Errors:        []int{http.StatusBadRequest, http.StatusNotFound, http.StatusGone, http.StatusUnprocessableEntity, http.StatusTooManyRequests},
})

func (h *messageHandler) sendMessage(ctx context.Context, in *sendChatMessageInput) (*sendChatMessageOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	message, replayed, err := h.chat.sendMessage(ctx, userID, sendChange{
		ConversationID: in.ID,
		Body:           in.Body.Body,
		AttachmentIDs:  in.Body.AttachmentIDs,
		NoPreview:      in.Body.NoPreview,
		Poll:           in.Body.Poll.change(),
		Key:            in.IdempotencyKey,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}

	out := &sendChatMessageOutput{Body: *message}
	if replayed {
		out.Replayed = "true"
	}
	return out, nil
}

type getChatMessageInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

type getChatMessageOutput struct {
	Body Message
}

var getChatMessageOperation = auth.Authenticated(huma.Operation{
	OperationID: "getChatMessage",
	Method:      http.MethodGet,
	Path:        "/chat/messages/{id}",
	Summary:     "Get one message with fresh attachment URLs",
	Tags:        []string{"chat"},
	Errors:      []int{http.StatusNotFound},
})

func (h *messageHandler) getMessage(ctx context.Context, in *getChatMessageInput) (*getChatMessageOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	message, err := h.chat.getMessage(ctx, viewerID, in.ID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &getChatMessageOutput{Body: *message}, nil
}

type deleteChatMessageInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

var deleteChatMessageOperation = auth.Authenticated(huma.Operation{
	OperationID:   "deleteChatMessage",
	Method:        http.MethodDelete,
	Path:          "/chat/messages/{id}",
	Summary:       "Delete one of the caller's messages for everyone",
	Description:   "The message stays as an empty tombstone so positions do not change; its text and images are removed. Deleting a deleted message succeeds.",
	Tags:          []string{"chat"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusForbidden, http.StatusNotFound},
})

func (h *messageHandler) deleteMessage(ctx context.Context, in *deleteChatMessageInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	if err := h.chat.deleteMessage(ctx, userID, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}
