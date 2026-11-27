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

type listPublishedEntriesInput struct {
	httpapi.PageQuery
	Lang string `query:"lang" enum:"en,pl,uk,ru,be,de,es" doc:"Language to read the entries in; defaults to the caller's profile language"`
}

type listEntriesOutput struct {
	Body struct {
		Items      []Entry `json:"items"`
		NextCursor *string `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get the next page; null on the last page"`
	}
}

var listEntriesOperation = auth.Authenticated(huma.Operation{
	OperationID: "listFaqEntries",
	Method:      http.MethodGet,
	Path:        "/faq",
	Summary:     "List published FAQ entries",
	Description: "Published entries only, newest first, in the lang query language or else the caller's profile language (an entry published before translations existed comes in its original language). A proposal or a rejected entry is never returned. An empty FAQ is an empty list.",
	Tags:        []string{"faq"},
	Errors:      []int{http.StatusBadRequest},
})

var publishEntryOperation = auth.Authenticated(huma.Operation{
	OperationID:   "publishFaqEntry",
	Method:        http.MethodPost,
	Path:          "/faq",
	Summary:       "Publish an FAQ entry",
	Description:   "Stores the question and answer in every interface language as published; a missing language is 400. Only one hardcoded address may call this. Everyone else gets 403.",
	Tags:          []string{"faq"},
	DefaultStatus: http.StatusCreated,
	Errors:        []int{http.StatusBadRequest, http.StatusForbidden},
})

type textsRequest struct {
	Texts map[string]EntryText `json:"texts" doc:"By two-letter language code; en, pl, uk, ru, be, de and es are all required"`
}

type publishEntryInput struct {
	Body textsRequest
}

type publishEntryOutput struct {
	Body Entry
}

func (h *entryHandler) publishEntry(ctx context.Context, in *publishEntryInput) (*publishEntryOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	entry, err := h.faq.publish(ctx, userID, in.Body.Texts)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &publishEntryOutput{Body: *entry}, nil
}

func (h *entryHandler) listEntries(ctx context.Context, in *listPublishedEntriesInput) (*listEntriesOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	return h.list(func() ([]Entry, *string, error) {
		return h.faq.listPublished(ctx, entryQuery{ViewerID: viewerID, Language: in.Lang, Cursor: in.Cursor, Limit: in.Limit})
	})
}

type proposalRequest struct {
	EntryText
	Language string `json:"language,omitempty" enum:"en,pl,uk,ru,be,de,es" doc:"Language the proposal is written in; defaults to the author's profile language"`
}

type createProposalInput struct {
	Body proposalRequest
}

type createProposalOutput struct {
	Body Entry
}

var createProposalOperation = auth.Authenticated(huma.Operation{
	OperationID:   "createFaqProposal",
	Method:        http.MethodPost,
	Path:          "/faq/proposals",
	Summary:       "Propose an FAQ entry",
	Description:   "Stores the question and answer as proposed, in one language. It stays out of the published list until a moderator translates and approves it.",
	Tags:          []string{"faq"},
	DefaultStatus: http.StatusCreated,
	Errors:        []int{http.StatusBadRequest},
})

func (h *entryHandler) createProposal(ctx context.Context, in *createProposalInput) (*createProposalOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	entry, err := h.faq.propose(ctx, proposalChange{AuthorID: userID, Language: in.Body.Language, Text: in.Body.EntryText})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &createProposalOutput{Body: *entry}, nil
}

var listProposalsOperation = auth.Authenticated(huma.Operation{
	OperationID: "listMyFaqProposals",
	Method:      http.MethodGet,
	Path:        "/faq/proposals",
	Summary:     "List the signed-in student's FAQ proposals",
	Description: "The caller's proposals only, newest first, including proposed, published and rejected, in the caller's profile language when the entry has it. Other students' proposals are absent.",
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
