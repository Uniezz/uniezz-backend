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
	ProfileID uuid.UUID `path:"profileId" format:"uuid"`
}

var blockStudentOperation = auth.Protected(huma.Operation{
	OperationID: "blockStudent",
	Method:      http.MethodPost,
	Path:        "/blocks/{profileId}",
	Summary:     "Block a student",
	Description: "A block hides each student from the other's discovery, profile list, profile and connections. Blocking twice changes nothing.",
	Tags:        []string{"connections"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *blockHandler) blockStudent(ctx context.Context, in *blockStudentInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	if err := h.connections.block(ctx, userID, in.ProfileID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return &struct{}{}, nil
}

type unblockStudentInput struct {
	ProfileID uuid.UUID `path:"profileId" format:"uuid"`
}

var unblockStudentOperation = auth.Protected(huma.Operation{
	OperationID: "unblockStudent",
	Method:      http.MethodDelete,
	Path:        "/blocks/{profileId}",
	Summary:     "Remove a block",
	Description: "Removing a block that does not exist changes nothing.",
	Tags:        []string{"connections"},
})

func (h *blockHandler) unblockStudent(ctx context.Context, in *unblockStudentInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	if err := h.connections.blocks.delete(ctx, userID, in.ProfileID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return &struct{}{}, nil
}
