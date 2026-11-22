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

type memberHandler struct {
	chat Module
}

type Member struct {
	Student  Student   `json:"student"`
	Role     Role      `json:"role" enum:"owner,member"`
	JoinedAt time.Time `json:"joinedAt"`
}

type listChatMembersInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
	httpapi.PageQuery
}

type listChatMembersOutput struct {
	Body struct {
		Items      []Member `json:"items"`
		NextCursor *string  `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get the next page; null on the last page"`
	}
}

var listChatMembersOperation = auth.Authenticated(huma.Operation{
	OperationID: "listChatMembers",
	Method:      http.MethodGet,
	Path:        "/chat/conversations/{id}/members",
	Summary:     "List the members of a conversation",
	Description: "The owner first, then by joining time. Students blocked either way or banned are left out.",
	Tags:        []string{"chat"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *memberHandler) listMembers(ctx context.Context, in *listChatMembersInput) (*listChatMembersOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	items, next, err := h.chat.listMembers(ctx, memberQuery{ViewerID: viewerID, ConversationID: in.ID, Cursor: in.Cursor, Limit: in.Limit})
	if err != nil {
		return nil, errorResponses.Error(err)
	}

	out := &listChatMembersOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

type addChatMembersInput struct {
	ID   uuid.UUID `path:"id" format:"uuid"`
	Body struct {
		UserIDs []uuid.UUID `json:"userIds" minItems:"1" maxItems:"49" uniqueItems:"true" doc:"Students to add, each one of your own Connections you have not blocked; members already in the group are skipped"`
	}
}

type addChatMembersOutput struct {
	Body struct {
		Items []Member `json:"items" doc:"The members that were added"`
	}
}

var addChatMembersOperation = auth.Authenticated(huma.Operation{
	OperationID:   "addChatMembers",
	Method:        http.MethodPost,
	Path:          "/chat/conversations/{id}/members",
	Summary:       "Add students to a group conversation",
	Description:   "Any member may add their own Connections. A group holds at most 50 members.",
	Tags:          []string{"chat"},
	DefaultStatus: http.StatusCreated,
	Errors:        []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict},
})

func (h *memberHandler) addMembers(ctx context.Context, in *addChatMembersInput) (*addChatMembersOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	items, err := h.chat.addMembers(ctx, userID, addChange{ConversationID: in.ID, UserIDs: in.Body.UserIDs})
	if err != nil {
		return nil, errorResponses.Error(err)
	}

	out := &addChatMembersOutput{}
	out.Body.Items = items
	return out, nil
}

type removeChatMemberInput struct {
	ID     uuid.UUID `path:"id" format:"uuid"`
	UserID uuid.UUID `path:"userId" format:"uuid"`
}

var removeChatMemberOperation = auth.Authenticated(huma.Operation{
	OperationID:   "removeChatMember",
	Method:        http.MethodDelete,
	Path:          "/chat/conversations/{id}/members/{userId}",
	Summary:       "Remove a student from a group conversation",
	Description:   "Only the owner may remove members; the owner leaves through leaveChatConversation.",
	Tags:          []string{"chat"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound},
})

func (h *memberHandler) removeMember(ctx context.Context, in *removeChatMemberInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	if err := h.chat.removeMember(ctx, userID, removeChange{ConversationID: in.ID, MemberID: in.UserID}); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}

type leaveChatConversationInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

var leaveChatConversationOperation = auth.Authenticated(huma.Operation{
	OperationID:   "leaveChatConversation",
	Method:        http.MethodDelete,
	Path:          "/chat/conversations/{id}/members/me",
	Summary:       "Leave a group conversation",
	Description:   "When the owner leaves, the longest-standing member becomes owner. The last member leaving deletes the group.",
	Tags:          []string{"chat"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *memberHandler) leaveConversation(ctx context.Context, in *leaveChatConversationInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	if err := h.chat.leaveConversation(ctx, userID, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}

type markChatReadInput struct {
	ID   uuid.UUID `path:"id" format:"uuid"`
	Body struct {
		Seq int64 `json:"seq" minimum:"0" doc:"The newest position the student has read, between 0 and the conversation's lastSeq. An older value than the current one changes nothing"`
	}
}

type markChatReadOutput struct {
	Body struct {
		LastReadSeq int64 `json:"lastReadSeq"`
		UnreadCount int   `json:"unreadCount"`
	}
}

var markChatReadOperation = auth.Authenticated(huma.Operation{
	OperationID: "markChatRead",
	Method:      http.MethodPut,
	Path:        "/chat/conversations/{id}/read",
	Summary:     "Set the read position in a conversation",
	Description: "The position only moves forward. The other participant of a direct conversation is told through the stream.",
	Tags:        []string{"chat"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *memberHandler) markRead(ctx context.Context, in *markChatReadInput) (*markChatReadOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	read, err := h.chat.markRead(ctx, userID, readChange{ConversationID: in.ID, Seq: in.Body.Seq})
	if err != nil {
		return nil, errorResponses.Error(err)
	}

	out := &markChatReadOutput{}
	out.Body.LastReadSeq = read.LastReadSeq
	out.Body.UnreadCount = read.UnreadCount
	return out, nil
}

type getChatUnreadOutput struct {
	Body struct {
		TotalUnread         int `json:"totalUnread" doc:"Unread messages over all visible conversations"`
		UnreadConversations int `json:"unreadConversations" doc:"Conversations with at least one unread message"`
	}
}

var getChatUnreadOperation = auth.Authenticated(huma.Operation{
	OperationID: "getChatUnread",
	Method:      http.MethodGet,
	Path:        "/chat/unread",
	Summary:     "Get the logged-in student's unread totals",
	Tags:        []string{"chat"},
})

func (h *memberHandler) getUnread(ctx context.Context, _ *struct{}) (*getChatUnreadOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	total, conversations, err := h.chat.unread(ctx, viewerID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}

	out := &getChatUnreadOutput{}
	out.Body.TotalUnread = total
	out.Body.UnreadConversations = conversations
	return out, nil
}
