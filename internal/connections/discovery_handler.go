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
	CandidateID     uuid.UUID `json:"candidateId" format:"uuid" doc:"Opaque token, valid only for this viewer"`
	Intent          *string   `json:"intent" nullable:"true"`
	Bio             *string   `json:"bio" nullable:"true"`
	Interests       []string  `json:"interests"`
	SharedInterests []string  `json:"sharedInterests"`
	UniversityID    string    `json:"universityId" example:"umcs"`
	Level           *string   `json:"level" nullable:"true" enum:"first_cycle,second_cycle,long_cycle,third_cycle,phd_school"`
	Age             *int      `json:"age" nullable:"true"`
}

type ActionResult struct {
	Matched    bool        `json:"matched"`
	Connection *Connection `json:"connection,omitempty" doc:"Present when this like created the match"`
}

type discoverInput struct {
	Mode       string   `query:"mode" required:"true" enum:"friends,studyBuddy,dating"`
	University []string `query:"university" maxItems:"10" doc:"Only students of these universities"`
	Faculty    string   `query:"faculty" maxLength:"255" doc:"studyBuddy only. Students who hide their faculty never match."`
	Level      []string `query:"level" enum:"first_cycle,second_cycle,long_cycle,third_cycle,phd_school" doc:"studyBuddy only"`
	Gender     string   `query:"gender" enum:"male,female,"`
	Interests  string   `query:"interests" enum:"shared," doc:"shared: only students with at least one interest in common"`
	Limit      int      `query:"limit" minimum:"1" maximum:"20" default:"1"`
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
	Description: "Both students must be open to the mode; dating shows only adults. A card never contains the user id, name, photos, faculty, programme or year. Liked, hidden, blocked and connected students are left out, and so are students passed in the last 30 days.",
	Tags:        []string{"connections"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *discoveryHandler) discover(ctx context.Context, in *discoverInput) (*discoverOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	items, err := h.connections.discover(ctx, discoverQuery{
		ViewerID:     userID,
		Mode:         in.Mode,
		Universities: in.University,
		Faculty:      in.Faculty,
		Levels:       in.Level,
		Gender:       in.Gender,
		Shared:       in.Interests == "shared",
		Limit:        in.Limit,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}

	out := &discoverOutput{}
	out.Body.Items = items
	return out, nil
}

type actOnCandidateInput struct {
	CandidateID uuid.UUID `path:"candidateId" format:"uuid"`
	Body        struct {
		Mode   string `json:"mode" enum:"friends,studyBuddy,dating"`
		Action string `json:"action" enum:"like,pass"`
	}
}

type actOnCandidateOutput struct {
	Body ActionResult
}

var actOnCandidateOperation = auth.Protected(huma.Operation{
	OperationID: "actOnCandidate",
	Method:      http.MethodPut,
	Path:        "/connections/candidates/{candidateId}",
	Summary:     "Like or pass a discovery card",
	Description: "A like cannot be undone; at most 50 likes per 24 hours. A pass hides the student for 30 days from the latest pass. Mutual likes in the same mode create one connection. Returns 404 when the candidateId belongs to another viewer, or when the two students can no longer meet in this mode: one left the mode, blocked the other, or they are already connected.",
	Tags:        []string{"connections"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound, http.StatusTooManyRequests},
})

func (h *discoveryHandler) actOnCandidate(ctx context.Context, in *actOnCandidateInput) (*actOnCandidateOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	result, err := h.connections.act(ctx, userID, in.CandidateID, in.Body.Mode, in.Body.Action)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &actOnCandidateOutput{Body: *result}, nil
}
