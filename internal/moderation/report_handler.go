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

type reportHandler struct {
	moderation Module
}

// reportTargetType is the kind of reported thing. New values are allowed:
// clients must ignore a targetType they do not know.
type reportTargetType string

func (reportTargetType) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	schema.Enum = nil
	if schema.Extensions == nil {
		schema.Extensions = map[string]any{}
	}
	// oasdiff treats this list as the enum. The enum keyword itself must stay
	// off the response, or adding a value is a breaking change.
	schema.Extensions["x-extensible-enum"] = []any{"post", "comment", "user", "candidate", "file", "course"}
	return schema
}

type Report struct {
	ID         uuid.UUID        `json:"id" format:"uuid"`
	TargetType reportTargetType `json:"targetType"`
	TargetID   uuid.UUID        `json:"targetId" format:"uuid"`
	Reason     string           `json:"reason" enum:"spam,harassment,inappropriate,hate_speech,other"`
	Details    *string          `json:"details" nullable:"true"`
	Status     string           `json:"status" enum:"pending,reviewed,dismissed"`
	CreatedAt  time.Time        `json:"createdAt"`
}

type createReportInput struct {
	Body struct {
		TargetType string    `json:"targetType" enum:"post,comment,user,candidate,file,course"`
		TargetID   uuid.UUID `json:"targetId" format:"uuid"`
		Reason     string    `json:"reason" enum:"spam,harassment,inappropriate,hate_speech,other"`
		Details    *string   `json:"details,omitempty" maxLength:"4000" doc:"Up to 1000 characters after HTML is stripped"`
	}
}

type createReportOutput struct {
	Body Report
}

var createReportOperation = auth.Protected(huma.Operation{
	OperationID:   "createReport",
	Method:        http.MethodPost,
	Path:          "/reports",
	Summary:       "Report a post, comment, student, exam file or course",
	Description:   "One report per target and reporter; at most 5 reports per clock hour (UTC).",
	Tags:          []string{"moderation"},
	DefaultStatus: http.StatusCreated,
	Errors:        []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests},
})

func (h *reportHandler) createReport(ctx context.Context, in *createReportInput) (*createReportOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	report, err := h.moderation.createReport(ctx, userID, reportInput{
		TargetType: in.Body.TargetType,
		TargetID:   in.Body.TargetID,
		Reason:     in.Body.Reason,
		Details:    in.Body.Details,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &createReportOutput{Body: *report}, nil
}

type listQueueInput struct {
	httpapi.PageQuery
}

type listQueueOutput struct {
	Body struct {
		Items      []QueuedReport `json:"items"`
		NextCursor *string        `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get the next page; null on the last page"`
	}
}

var listQueueOperation = auth.Protected(huma.Operation{
	OperationID: "listPendingReports",
	Method:      http.MethodGet,
	Path:        "/moderation/reports",
	Summary:     "List pending reports for moderators",
	Description: "Pending reports about posts, comments, students, exam files and courses, newest first. Moderators only; everyone else gets 403.",
	Tags:        []string{"moderation"},
	Errors:      []int{http.StatusBadRequest, http.StatusForbidden},
})

func (h *reportHandler) listQueue(ctx context.Context, in *listQueueInput) (*listQueueOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	items, next, err := h.moderation.listQueue(ctx, userID, in.Cursor, in.Limit)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &listQueueOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}
