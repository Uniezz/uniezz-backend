package moderation

import (
	"context"
	"net/http"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type moderatorHandler struct {
	moderation Module
}

type Moderator struct {
	UserID    uuid.UUID `json:"userId" format:"uuid"`
	FirstName *string   `json:"firstName" nullable:"true" doc:"Null until the student sets a name" example:"Jan"`
	LastName  *string   `json:"lastName" nullable:"true" doc:"Null until the student sets a name" example:"Kowalski"`
	GrantedAt time.Time `json:"grantedAt"`
}

type moderatorInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

type listModeratorsInput struct {
	httpapi.PageQuery
}

type listModeratorsOutput struct {
	Body struct {
		Items      []Moderator `json:"items"`
		NextCursor *string     `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get the next page; null on the last page"`
	}
}

var listModeratorsOperation = auth.Authorized(huma.Operation{
	OperationID: "listModerators",
	Method:      http.MethodGet,
	Path:        "/moderation/moderators",
	Summary:     "List the students who are moderators",
	Description: "Newest moderator first. Admins only.",
	Tags:        []string{"moderation"},
	Errors:      []int{http.StatusBadRequest},
}, auth.RoleAdmin)

func (h *moderatorHandler) listModerators(ctx context.Context, in *listModeratorsInput) (*listModeratorsOutput, error) {
	items, next, err := h.moderation.listModerators(ctx, moderatorQuery{Cursor: in.Cursor, Limit: in.Limit})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &listModeratorsOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

var grantModeratorOperation = auth.Authorized(huma.Operation{
	OperationID:   "grantModerator",
	Method:        http.MethodPut,
	Path:          "/moderation/moderators/{id}",
	Summary:       "Make a student a moderator",
	Description:   "Takes effect on the student's next request, with no new sign-in. Granting it again changes nothing. Admins only.",
	Tags:          []string{"moderation"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusNotFound},
}, auth.RoleAdmin)

func (h *moderatorHandler) grantModerator(ctx context.Context, in *moderatorInput) (*struct{}, error) {
	if err := h.moderation.grantModerator(ctx, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}

var revokeModeratorOperation = auth.Authorized(huma.Operation{
	OperationID:   "revokeModerator",
	Method:        http.MethodDelete,
	Path:          "/moderation/moderators/{id}",
	Summary:       "Stop a student being a moderator",
	Description:   "Takes effect on the student's next request. Revoking from a student who is not a moderator succeeds. Admins only.",
	Tags:          []string{"moderation"},
	DefaultStatus: http.StatusNoContent,
}, auth.RoleAdmin)

func (h *moderatorHandler) revokeModerator(ctx context.Context, in *moderatorInput) (*struct{}, error) {
	if err := h.moderation.revokeModerator(ctx, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}
