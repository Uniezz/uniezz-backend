package connections

import (
	"context"
	"net/http"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/profile"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type connectionHandler struct {
	connections Module
}

type Connection struct {
	ID          uuid.UUID              `json:"id" format:"uuid"`
	Mode        Mode                   `json:"mode" enum:"friends,studyBuddy,dating"`
	ConnectedAt time.Time              `json:"connectedAt"`
	Profile     profile.ProfileSummary `json:"profile"`
}

type listConnectionsInput struct {
	httpapi.PageQuery
}

type listConnectionsOutput struct {
	Body struct {
		Items      []Connection `json:"items"`
		NextCursor *string      `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get the next page; null on the last page"`
	}
}

var listConnectionsOperation = auth.Protected(huma.Operation{
	OperationID: "listConnections",
	Method:      http.MethodGet,
	Path:        "/connections",
	Summary:     "List the logged-in student's matches",
	Description: "Newest first, paginated by cursor. Students blocked either way are left out.",
	Tags:        []string{"connections"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *connectionHandler) listConnections(ctx context.Context, in *listConnectionsInput) (*listConnectionsOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	items, next, err := h.connections.list(ctx, userID, in.Cursor, in.Limit)
	if err != nil {
		return nil, errorResponses.Error(err)
	}

	out := &listConnectionsOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

type deleteConnectionInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

var deleteConnectionOperation = auth.Protected(huma.Operation{
	OperationID:   "deleteConnection",
	Method:        http.MethodDelete,
	Path:          "/connections/{id}",
	Summary:       "Remove a match for both students",
	Tags:          []string{"connections"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusNotFound},
})

func (h *connectionHandler) deleteConnection(ctx context.Context, in *deleteConnectionInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	if err := h.connections.deleteConnection(ctx, userID, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}
