package moderation

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type queueHandler struct {
	moderation Module
}

// QueuedReport is a report as a moderator sees it: the public fields plus the
// copy of the reported content. It never names the reporter.
type QueuedReport struct {
	Report
	TargetAuthorID *uuid.UUID     `json:"targetAuthorId" format:"uuid" nullable:"true" doc:"Null when the content has no author, or once the author's data has been anonymised"`
	Snapshot       map[string]any `json:"snapshot" doc:"Copy of the reported content at the time of the report"`
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

var listQueueOperation = auth.Authorized(huma.Operation{
	OperationID: "listPendingReports",
	Method:      http.MethodGet,
	Path:        "/moderation/reports",
	Summary:     "List pending reports for moderators",
	Description: "Pending reports about posts, comments, students, exam files and courses, newest first. Moderators and admins only; everyone else gets 403.",
	Tags:        []string{"moderation"},
	Errors:      []int{http.StatusBadRequest},
}, auth.RoleModerator, auth.RoleAdmin)

func (h *queueHandler) listQueue(ctx context.Context, in *listQueueInput) (*listQueueOutput, error) {
	items, next, err := h.moderation.listQueue(ctx, queueQuery{Cursor: in.Cursor, Limit: in.Limit})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &listQueueOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

type getReportInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

type getReportOutput struct {
	Body QueuedReport
}

var getReportOperation = auth.Authorized(huma.Operation{
	OperationID: "getReport",
	Method:      http.MethodGet,
	Path:        "/moderation/reports/{id}",
	Summary:     "Get a report about a post, comment, student, exam file or course",
	Description: "Any status, with the copy of the reported content. Moderators and admins only.",
	Tags:        []string{"moderation"},
	Errors:      []int{http.StatusNotFound},
}, auth.RoleModerator, auth.RoleAdmin)

func (h *queueHandler) getReport(ctx context.Context, in *getReportInput) (*getReportOutput, error) {
	report, err := h.moderation.report(ctx, in.ID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &getReportOutput{Body: *report}, nil
}

type updateReportInput struct {
	ID   uuid.UUID `path:"id" format:"uuid"`
	Body struct {
		Status ReportStatus `json:"status" enum:"reviewed,dismissed"`
	}
}

type updateReportOutput struct {
	Body Report
}

var updateReportOperation = auth.Authorized(huma.Operation{
	OperationID: "updateReport",
	Method:      http.MethodPatch,
	Path:        "/moderation/reports/{id}",
	Summary:     "Resolve a report about a post, comment, student, exam file or course",
	Description: "Sets the status of this report and of every other pending report on the same target, so one decision clears them all. Sending the current status again changes nothing. Moderators and admins only.",
	Tags:        []string{"moderation"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
}, auth.RoleModerator, auth.RoleAdmin)

func (h *queueHandler) updateReport(ctx context.Context, in *updateReportInput) (*updateReportOutput, error) {
	report, err := h.moderation.updateReport(ctx, in.ID, reportChange{Status: in.Body.Status})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &updateReportOutput{Body: *report}, nil
}
