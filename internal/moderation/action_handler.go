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

type actionHandler struct {
	moderation Module
}

func (ActionType) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	schema.Enum = nil
	if schema.Extensions == nil {
		schema.Extensions = map[string]any{}
	}
	schema.Extensions["x-extensible-enum"] = []any{
		ActionReviewReport, ActionDismissReport, ActionRemovePost, ActionRemoveComment, ActionRemoveFile,
		ActionRemoveCourse, ActionRemoveCourseComment, ActionRemoveProfessorReview, ActionGrantModerator, ActionRevokeModerator, ActionBan, ActionUnban,
	}
	return schema
}

type Action struct {
	ID        uuid.UUID  `json:"id" format:"uuid"`
	ActorID   *uuid.UUID `json:"actorId" format:"uuid" nullable:"true" doc:"The moderator or admin who acted; null once their account is deleted"`
	Type      ActionType `json:"type" doc:"Reports are the target of review_report and dismiss_report, students of grant_moderator, revoke_moderator, ban and unban, and the named content of the remove actions"`
	TargetID  uuid.UUID  `json:"targetId" format:"uuid"`
	CreatedAt time.Time  `json:"createdAt"`
}

type listActionsInput struct {
	httpapi.PageQuery
}

type listActionsOutput struct {
	Body struct {
		Items      []Action `json:"items"`
		NextCursor *string  `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get the next page; null on the last page"`
	}
}

var listActionsOperation = auth.Authorized(huma.Operation{
	OperationID: "listModerationActions",
	Method:      http.MethodGet,
	Path:        "/moderation/actions",
	Summary:     "List what moderators and admins did",
	Description: "Newest first; actions older than a year are deleted. A repeated request that changed nothing is listed again. Admins only.",
	Tags:        []string{"moderation"},
	Errors:      []int{http.StatusBadRequest},
}, auth.RoleAdmin)

func (h *actionHandler) listActions(ctx context.Context, in *listActionsInput) (*listActionsOutput, error) {
	items, next, err := h.moderation.listActions(ctx, actionQuery{Cursor: in.Cursor, Limit: in.Limit})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &listActionsOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}
