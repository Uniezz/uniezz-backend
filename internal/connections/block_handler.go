package connections

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type blockHandler struct {
	connections Module
}

type blockStudentInput struct {
	ID uuid.UUID `path:"id" format:"uuid" doc:"The blocked student's profile id"`
}

var blockStudentOperation = auth.Protected(huma.Operation{
	OperationID:   "blockStudent",
	Method:        http.MethodPut,
	Path:          "/connections/blocks/{id}",
	Summary:       "Block a student",
	Description:   "Each student stops seeing the other in discovery, the profile list, the profile, connections, and the feed: posts, comments and the featured list. Blocking twice changes nothing.",
	Tags:          []string{"connections"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *blockHandler) blockStudent(ctx context.Context, in *blockStudentInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	if err := h.connections.block(ctx, userID, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}

type unblockStudentInput struct {
	ID uuid.UUID `path:"id" format:"uuid" doc:"The blocked student's profile id"`
}

var unblockStudentOperation = auth.Protected(huma.Operation{
	OperationID:   "unblockStudent",
	Method:        http.MethodDelete,
	Path:          "/connections/blocks/{id}",
	Summary:       "Remove a block",
	Description:   "Succeeds when there is no such block.",
	Tags:          []string{"connections"},
	DefaultStatus: http.StatusNoContent,
})

func (h *blockHandler) unblockStudent(ctx context.Context, in *unblockStudentInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	if err := h.connections.unblock(ctx, userID, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}
