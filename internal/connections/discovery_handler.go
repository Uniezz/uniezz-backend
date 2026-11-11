package connections

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type discoveryHandler struct {
	connections Module
}

type Card struct {
	CandidateID     uuid.UUID `json:"candidateId" format:"uuid" doc:"Opaque token, valid only for this viewer, for 30 days after it was first issued"`
	Intent          *string   `json:"intent" nullable:"true" doc:"Null when the student has none"`
	Bio             *string   `json:"bio" nullable:"true" doc:"Null when the student has none"`
	Interests       []string  `json:"interests"`
	SharedInterests []string  `json:"sharedInterests" doc:"The interests the viewer also has"`
	UniversityID    string    `json:"universityId" example:"umcs"`
	Level           *string   `json:"level" nullable:"true" enum:"first_cycle,second_cycle,long_cycle,third_cycle,phd_school" doc:"Of the primary programme; null when unknown"`
	Age             *int      `json:"age" nullable:"true" doc:"Null when the student has not set a birth date"`
}

type ActionResult struct {
	Matched    bool        `json:"matched"`
	Connection *Connection `json:"connection,omitempty" doc:"Present when this like created the match"`
}

type discoverInput struct {
	Mode            Mode     `query:"mode" required:"true" enum:"friends,studyBuddy,dating"`
	University      []string `query:"university" maxItems:"10" doc:"Only students of these universities; repeat the parameter for several" example:"umcs"`
	Faculty         string   `query:"faculty" maxLength:"255" doc:"studyBuddy only. Students who hide their faculty never match."`
	Level           []string `query:"level" enum:"first_cycle,second_cycle,long_cycle,third_cycle,phd_school" maxItems:"5" doc:"studyBuddy only"`
	Gender          string   `query:"gender" enum:"male,female"`
	SharedInterests bool     `query:"sharedInterests" doc:"Only students with at least one interest in common"`
	Limit           int      `query:"limit" minimum:"1" maximum:"20" default:"1"`
}

type discoverOutput struct {
	Body struct {
		Items []Card `json:"items"`
	}
}

var discoverOperation = auth.Protected(huma.Operation{
	OperationID: "discoverConnections",
	Method:      http.MethodGet,
	Path:        "/connections/discover",
	Summary:     "Discover anonymous students",
	Description: "A deck of up to limit cards, best match first, with no cursor: liked, hidden, blocked and connected students and those passed in the last 30 days are left out, so the next call shows the next cards. Both students must be open to the mode, and dating shows only adults. A card never contains the user id, name, photos, faculty, programme or year; issuing its candidateId is the only write.",
	Tags:        []string{"connections"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *discoveryHandler) discover(ctx context.Context, in *discoverInput) (*discoverOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	items, err := h.connections.discover(ctx, discoveryQuery{
		ViewerID:        userID,
		Mode:            in.Mode,
		Universities:    in.University,
		Faculty:         in.Faculty,
		Levels:          in.Level,
		Gender:          in.Gender,
		SharedInterests: in.SharedInterests,
		Limit:           in.Limit,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}

	out := &discoverOutput{}
	out.Body.Items = items
	return out, nil
}

type actOnCandidateInput struct {
	ID   uuid.UUID `path:"id" format:"uuid"`
	Body struct {
		Mode   Mode   `json:"mode" enum:"friends,studyBuddy,dating"`
		Action action `json:"action" enum:"like,pass"`
	}
}

type actOnCandidateOutput struct {
	Body ActionResult
}

var actOnCandidateOperation = auth.Protected(huma.Operation{
	OperationID: "actOnCandidate",
	Method:      http.MethodPut,
	Path:        "/connections/candidates/{id}",
	Summary:     "Like or pass a discovery card",
	Description: "A like cannot be undone; at most 50 likes per 24 hours. A pass hides the student for 30 days from the latest pass. Mutual likes in the same mode create one connection. Returns 404 when the candidate id belongs to another viewer or has expired, or when the two students can no longer meet in this mode.",
	Tags:        []string{"connections"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound, http.StatusTooManyRequests},
})

func (h *discoveryHandler) actOnCandidate(ctx context.Context, in *actOnCandidateInput) (*actOnCandidateOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	result, err := h.connections.act(ctx, userID, in.ID, in.Body.Mode, in.Body.Action)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &actOnCandidateOutput{Body: *result}, nil
}
