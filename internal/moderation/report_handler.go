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
	if schema.Extensions == nil {
		schema.Extensions = map[string]any{}
	}
	schema.Extensions["x-extensible-enum"] = true
	return schema
}

type Report struct {
	ID         uuid.UUID        `json:"id" format:"uuid"`
	TargetType reportTargetType `json:"targetType" enum:"post,comment,user,candidate"`
	TargetID   uuid.UUID        `json:"targetId" format:"uuid"`
	Reason     string           `json:"reason" enum:"spam,harassment,inappropriate,hate_speech,other"`
	Details    *string          `json:"details" nullable:"true"`
	Status     string           `json:"status" enum:"pending,reviewed,dismissed"`
	CreatedAt  time.Time        `json:"createdAt"`
}

type createReportInput struct {
	Body struct {
		TargetType reportTargetType `json:"targetType" enum:"post,comment,user,candidate"`
		TargetID   uuid.UUID        `json:"targetId" format:"uuid"`
		Reason     string           `json:"reason" enum:"spam,harassment,inappropriate,hate_speech,other"`
		Details    *string          `json:"details,omitempty" maxLength:"4000" doc:"Up to 1000 characters after HTML is stripped"`
	}
}

type createReportOutput struct {
	Body Report
}

var createReportOperation = auth.Protected(huma.Operation{
	OperationID:   "createReport",
	Method:        http.MethodPost,
	Path:          "/reports",
	Summary:       "Report a post, comment or student",
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
		TargetType: string(in.Body.TargetType),
		TargetID:   in.Body.TargetID,
		Reason:     in.Body.Reason,
		Details:    in.Body.Details,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &createReportOutput{Body: *report}, nil
}
