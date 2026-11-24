package chat

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type requestHandler struct {
	chat Module
}

type listChatRequestsInput struct {
	httpapi.PageQuery
}

type listChatRequestsOutput struct {
	Body struct {
		Items      []Conversation `json:"items"`
		NextCursor *string        `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get the next page; null on the last page"`
	}
}

var listChatRequestsOperation = auth.Authenticated(huma.Operation{
	OperationID: "listChatRequests",
	Method:      http.MethodGet,
	Path:        "/chat/requests",
	Summary:     "List the message requests sent to the logged-in student",
	Description: "Pending requests that hold a message, newest first, paginated by cursor. Declined requests and requests from students blocked either way are left out.",
	Tags:        []string{"chat"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *requestHandler) listRequests(ctx context.Context, in *listChatRequestsInput) (*listChatRequestsOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	items, next, err := h.chat.listConversations(ctx, conversationQuery{ViewerID: viewerID, Requests: true, Cursor: in.Cursor, Limit: in.Limit})
	if err != nil {
		return nil, errorResponses.Error(err)
	}

	out := &listChatRequestsOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

type acceptChatRequestInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

type acceptChatRequestOutput struct {
	Body Conversation
}

var acceptChatRequestOperation = auth.Authenticated(huma.Operation{
	OperationID: "acceptChatRequest",
	Method:      http.MethodPut,
	Path:        "/chat/requests/{id}/acceptance",
	Summary:     "Accept a message request sent to the logged-in student",
	Description: "Turns the request into a normal conversation; replying to it does the same. Accepting it again changes nothing.",
	Tags:        []string{"chat"},
	Errors:      []int{http.StatusNotFound},
})

func (h *requestHandler) acceptRequest(ctx context.Context, in *acceptChatRequestInput) (*acceptChatRequestOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	conversation, err := h.chat.acceptRequest(ctx, viewerID, in.ID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &acceptChatRequestOutput{Body: *conversation}, nil
}

type declineChatRequestInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

var declineChatRequestOperation = auth.Authenticated(huma.Operation{
	OperationID:   "declineChatRequest",
	Method:        http.MethodDelete,
	Path:          "/chat/requests/{id}",
	Summary:       "Decline a message request sent to the logged-in student",
	Description:   "The request disappears for the caller; its sender is not told and still sees it as pending. Declining it again succeeds.",
	Tags:          []string{"chat"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusNotFound},
})

func (h *requestHandler) declineRequest(ctx context.Context, in *declineChatRequestInput) (*struct{}, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	if err := h.chat.declineRequest(ctx, viewerID, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}
