package faq

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type approvalHandler struct {
	faq Module
}

type listPendingProposalsInput struct {
	httpapi.PageQuery
}

type listPendingProposalsOutput struct {
	Body struct {
		Items      []Entry `json:"items"`
		NextCursor *string `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get the next page; null on the last page"`
	}
}

var listPendingProposalsOperation = auth.Authorized(huma.Operation{
	OperationID: "listPendingFaqProposals",
	Method:      http.MethodGet,
	Path:        "/faq/proposals/pending",
	Summary:     "List FAQ proposals waiting for a moderator",
	Description: "Every student's proposals that are not published yet, newest first. Moderators and admins only; everyone else gets 403.",
	Tags:        []string{"faq"},
	Errors:      []int{http.StatusBadRequest},
}, auth.RoleModerator, auth.RoleAdmin)

func (h *approvalHandler) listPendingProposals(ctx context.Context, in *listPendingProposalsInput) (*listPendingProposalsOutput, error) {
	items, next, err := h.faq.listPending(ctx, in.Cursor, in.Limit)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &listPendingProposalsOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

type approveProposalInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

type approveProposalOutput struct {
	Body Entry
}

var approveProposalOperation = auth.Authorized(huma.Operation{
	OperationID: "approveFaqProposal",
	Method:      http.MethodPut,
	Path:        "/faq/proposals/{id}/approval",
	Summary:     "Approve an FAQ proposal and publish it",
	Description: "Moderators and admins only. Approving pays the author 5 Uniezz coins once, unless the moderator approves their own proposal; approving an entry that is already published changes nothing. A rejected or unknown entry is 404.",
	Tags:        []string{"faq"},
	Errors:      []int{http.StatusNotFound},
}, auth.RoleModerator, auth.RoleAdmin)

func (h *approvalHandler) approveProposal(ctx context.Context, in *approveProposalInput) (*approveProposalOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	entry, err := h.faq.approve(ctx, userID, in.ID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &approveProposalOutput{Body: *entry}, nil
}
