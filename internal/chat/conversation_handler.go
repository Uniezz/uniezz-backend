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

type conversationHandler struct {
	chat Module
}

type Kind string

const (
	KindDirect Kind = "direct"
	KindGroup  Kind = "group"
)

type Role string

const (
	RoleOwner  Role = "owner"
	RoleMember Role = "member"
)

type Origin string

const (
	OriginMatch   Origin = "match"
	OriginRequest Origin = "request"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusDeclined Status = "declined"
)

type Student struct {
	ID           *uuid.UUID `json:"id" format:"uuid" nullable:"true" doc:"Null when the student deleted their account"`
	Deleted      bool       `json:"deleted" doc:"true when the student deleted their account"`
	FirstName    *string    `json:"firstName" nullable:"true" example:"Jan"`
	LastName     *string    `json:"lastName" nullable:"true" example:"Kowalski"`
	UniversityID *string    `json:"universityId" nullable:"true" example:"umcs"`
	AvatarURL    *string    `json:"avatarUrl" nullable:"true"`
}

type Conversation struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	Kind        Kind      `json:"kind" enum:"direct,group"`
	Name        *string   `json:"name" nullable:"true" doc:"Null for direct conversations"`
	LastSeq     int64     `json:"lastSeq" doc:"Position of the newest message"`
	ChangeSeq   int64     `json:"changeSeq" doc:"Sync cursor: pass the highest value you hold as changedAfter to list what changed since"`
	LastReadSeq int64     `json:"lastReadSeq"`
	UnreadCount int       `json:"unreadCount"`
	LastMessage *Message  `json:"lastMessage,omitempty" doc:"Newest message the viewer may see; omitted when there is none"`
	Peer        *Student  `json:"peer,omitempty" doc:"The other student of a direct conversation; omitted for groups"`
	MemberCount *int      `json:"memberCount" nullable:"true" doc:"Null for direct conversations"`
	MyRole      *Role     `json:"myRole" nullable:"true" enum:"owner,member" doc:"Null for direct conversations"`
	Origin      *Origin   `json:"origin" nullable:"true" enum:"match,request" doc:"How the direct conversation began; request stays even after the students match. Null for groups"`
	Status      *Status   `json:"status" nullable:"true" enum:"pending,approved" doc:"pending until the recipient accepts or replies; a declined request still shows pending to its sender. Null for groups"`
	Incoming    *bool     `json:"incoming" nullable:"true" doc:"true when the other student started the direct conversation. Null for groups"`
	CreatedAt   time.Time `json:"createdAt"`
}

type openDirectConversationInput struct {
	Body struct {
		UserID uuid.UUID `json:"userId" format:"uuid" doc:"Any student; without a Connection the conversation starts as a message request"`
	}
}

type openDirectConversationOutput struct {
	Status int
	Body   Conversation
}

var openDirectConversationOperation = auth.Authenticated(huma.Operation{
	OperationID:   "openDirectConversation",
	Method:        http.MethodPost,
	Path:          "/chat/conversations/direct",
	Summary:       "Open the direct conversation with a student",
	Description:   "Returns 201 when the conversation was created and 200 when it already existed. Without a Connection it is a message request: the caller may send one text message without links until the other student accepts or replies, at most 20 new requests a day, counted at their first message. Students who do not accept messages from the caller, blocked either way, banned or unknown answer 404.",
	Tags:          []string{"chat"},
	DefaultStatus: http.StatusCreated,
	Errors:        []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *conversationHandler) openDirectConversation(ctx context.Context, in *openDirectConversationInput) (*openDirectConversationOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	conversation, created, err := h.chat.openDirect(ctx, viewerID, in.Body.UserID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}

	out := &openDirectConversationOutput{Status: http.StatusOK, Body: *conversation}
	if created {
		out.Status = http.StatusCreated
	}
	return out, nil
}

type groupRequest struct {
	Name      string      `json:"name" maxLength:"400" doc:"1–80 characters after HTML is stripped" example:"Informatyka rok 2"`
	MemberIDs []uuid.UUID `json:"memberIds" minItems:"1" maxItems:"49" uniqueItems:"true" doc:"Students to add, each one of your Connections you have not blocked"`
}

type createGroupConversationInput struct {
	IdempotencyKey string `header:"Idempotency-Key" minLength:"36" maxLength:"36" doc:"A UUID. Repeating the request with the same key returns the group the first one created instead of creating another"`
	Body           groupRequest
}

type createGroupConversationOutput struct {
	Replayed string `header:"Idempotent-Replayed" doc:"true when the group was created by an earlier request with the same key"`
	Body     Conversation
}

var createGroupConversationOperation = auth.Authenticated(huma.Operation{
	OperationID:   "createGroupConversation",
	Method:        http.MethodPost,
	Path:          "/chat/conversations/groups",
	Summary:       "Create a group conversation",
	Description:   "The caller becomes the owner. At most 5 groups per hour and 50 members including the owner.",
	Tags:          []string{"chat"},
	DefaultStatus: http.StatusCreated,
	Errors:        []int{http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusTooManyRequests},
})

func (h *conversationHandler) createGroupConversation(ctx context.Context, in *createGroupConversationInput) (*createGroupConversationOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	conversation, replayed, err := h.chat.createGroup(ctx, userID, groupChange{
		Name:      in.Body.Name,
		MemberIDs: in.Body.MemberIDs,
		Key:       in.IdempotencyKey,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}

	out := &createGroupConversationOutput{Body: *conversation}
	if replayed {
		out.Replayed = "true"
	}
	return out, nil
}

type listChatConversationsInput struct {
	httpapi.PageQuery
}

type listChatConversationsOutput struct {
	Body struct {
		Items      []Conversation `json:"items"`
		NextCursor *string        `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get the next page; null on the last page"`
	}
}

var listChatConversationsOperation = auth.Authenticated(huma.Operation{
	OperationID: "listChatConversations",
	Method:      http.MethodGet,
	Path:        "/chat/conversations",
	Summary:     "List the logged-in student's conversations",
	Description: "Newest activity first, paginated by cursor. Message requests the caller has not accepted yet, the caller's own requests until they hold a message, and direct conversations with a block either way, are left out.",
	Tags:        []string{"chat"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *conversationHandler) listConversations(ctx context.Context, in *listChatConversationsInput) (*listChatConversationsOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	items, next, err := h.chat.listConversations(ctx, conversationQuery{ViewerID: viewerID, Cursor: in.Cursor, Limit: in.Limit})
	if err != nil {
		return nil, errorResponses.Error(err)
	}

	out := &listChatConversationsOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

type getChatConversationInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

type getChatConversationOutput struct {
	Body Conversation
}

var getChatConversationOperation = auth.Authenticated(huma.Operation{
	OperationID: "getChatConversation",
	Method:      http.MethodGet,
	Path:        "/chat/conversations/{id}",
	Summary:     "Get one of the logged-in student's conversations",
	Tags:        []string{"chat"},
	Errors:      []int{http.StatusNotFound},
})

func (h *conversationHandler) getConversation(ctx context.Context, in *getChatConversationInput) (*getChatConversationOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	conversation, err := h.chat.getConversation(ctx, viewerID, in.ID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &getChatConversationOutput{Body: *conversation}, nil
}

type updateChatConversationInput struct {
	ID   uuid.UUID `path:"id" format:"uuid"`
	Body struct {
		Name string `json:"name" maxLength:"400" doc:"1–80 characters after HTML is stripped" example:"Informatyka rok 2"`
	}
}

type updateChatConversationOutput struct {
	Body Conversation
}

var updateChatConversationOperation = auth.Authenticated(huma.Operation{
	OperationID: "updateChatConversation",
	Method:      http.MethodPatch,
	Path:        "/chat/conversations/{id}",
	Summary:     "Rename a group conversation",
	Description: "Only the owner may rename a group; the other members get a system message. Sending the current name again changes nothing.",
	Tags:        []string{"chat"},
	Errors:      []int{http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound},
})

func (h *conversationHandler) updateConversation(ctx context.Context, in *updateChatConversationInput) (*updateChatConversationOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	conversation, err := h.chat.renameConversation(ctx, userID, renameChange{ConversationID: in.ID, Name: in.Body.Name})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &updateChatConversationOutput{Body: *conversation}, nil
}
