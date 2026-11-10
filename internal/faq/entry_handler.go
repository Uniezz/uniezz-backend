package faq

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
)

type entryHandler struct {
	faq Module
}

type listEntriesInput struct {
	httpapi.PageQuery
}

type listEntriesOutput struct {
	Body struct {
		Items      []Entry `json:"items"`
		NextCursor *string `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get the next page; null on the last page"`
	}
}

var listEntriesOperation = auth.Protected(huma.Operation{
	OperationID: "listFaqEntries",
	Method:      http.MethodGet,
	Path:        "/faq",
	Summary:     "List published FAQ entries",
	Description: "Published entries only, newest first. A proposal or a rejected entry is never returned. An empty FAQ is an empty list.",
	Tags:        []string{"faq"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *entryHandler) listEntries(ctx context.Context, in *listEntriesInput) (*listEntriesOutput, error) {
	if _, ok := auth.UserIDFrom(ctx); !ok {
		return nil, httpapi.Unauthorized()
	}
	return h.list(func() ([]Entry, *string, error) {
		return h.faq.listPublished(ctx, in.Cursor, in.Limit)
	})
}

type proposalRequest struct {
	Question string `json:"question" minLength:"1" maxLength:"1000" doc:"3 to 200 characters after HTML is stripped"`
	Answer   string `json:"answer" minLength:"1" maxLength:"8000" doc:"1 to 2000 characters after HTML is stripped"`
}

type createProposalInput struct {
	Body proposalRequest
}

type createProposalOutput struct {
	Body Entry
}

var createProposalOperation = auth.Protected(huma.Operation{
	OperationID:   "createFaqProposal",
	Method:        http.MethodPost,
	Path:          "/faq/proposals",
	Summary:       "Propose an FAQ entry",
	Description:   "Stores the question and answer as proposed. It stays out of the published list until a moderator decides.",
	Tags:          []string{"faq"},
	DefaultStatus: http.StatusCreated,
	Errors:        []int{http.StatusBadRequest},
})

func (h *entryHandler) createProposal(ctx context.Context, in *createProposalInput) (*createProposalOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	entry, err := h.faq.propose(ctx, userID, in.Body.Question, in.Body.Answer)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &createProposalOutput{Body: *entry}, nil
}

var listProposalsOperation = auth.Protected(huma.Operation{
	OperationID: "listMyFaqProposals",
	Method:      http.MethodGet,
	Path:        "/faq/proposals",
	Summary:     "List the signed-in student's FAQ proposals",
	Description: "The caller's proposals only, newest first, including proposed, published and rejected. Other students' proposals are absent.",
	Tags:        []string{"faq"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *entryHandler) listProposals(ctx context.Context, in *listEntriesInput) (*listEntriesOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	return h.list(func() ([]Entry, *string, error) {
		return h.faq.listMine(ctx, userID, in.Cursor, in.Limit)
	})
}

func (h *entryHandler) list(load func() ([]Entry, *string, error)) (*listEntriesOutput, error) {
	items, next, err := load()
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &listEntriesOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}
