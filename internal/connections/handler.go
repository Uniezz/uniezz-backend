package connections

import (
	"context"
	"net/http"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type handler struct {
	connections Module
}

type preferencesInput struct {
	Body struct {
		BirthDate string   `json:"birthDate,omitempty" format:"date" doc:"YYYY-MM-DD"`
		OpenTo    []string `json:"openTo,omitempty" doc:"friends, studyBuddy, dating"`
		Intent    *string  `json:"intent,omitempty" maxLength:"200"`
	}
}

var savePreferencesOperation = auth.Protected(huma.Operation{
	OperationID: "saveConnectionPreferences",
	Method:      http.MethodPatch,
	Path:        "/connections/me",
	Summary:     "Set what Connections may show",
	Description: "Birth date is used only to compute age. openTo is who the student can discover and be discovered as.",
	Tags:        []string{"connections"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *handler) savePreferences(ctx context.Context, in *preferencesInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	var birth *time.Time
	if in.Body.BirthDate != "" {
		parsed, err := time.Parse("2006-01-02", in.Body.BirthDate)
		if err != nil {
			return nil, errorResponses.Error(ErrInvalidInput)
		}
		birth = &parsed
	}
	err := h.connections.savePreferences(ctx, userID, birth, in.Body.OpenTo, in.Body.Intent, in.Body.Intent != nil)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &struct{}{}, nil
}

type discoverInput struct {
	Mode       string   `query:"mode" required:"true" enum:"friends,studyBuddy,dating"`
	University []string `query:"university"`
	Faculty    string   `query:"faculty"`
	Level      []string `query:"level"`
	Gender     string   `query:"gender" enum:"male,female,"`
	Interests  string   `query:"interests" enum:"shared,"`
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
	Description: "A card never contains the user id, name, photos, faculty, programme or year. candidateId is an opaque token for this viewer.",
	Tags:        []string{"connections"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *handler) discover(ctx context.Context, in *discoverInput) (*discoverOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	items, err := h.connections.discover(ctx, discoverQuery{
		Viewer: userID, Mode: in.Mode, University: in.University, Faculty: in.Faculty,
		Levels: in.Level, Gender: in.Gender, Shared: in.Interests == "shared", Limit: in.Limit,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &discoverOutput{}
	out.Body.Items = items
	return out, nil
}

type actInput struct {
	CandidateID uuid.UUID `path:"candidateId" format:"uuid"`
	Body        struct {
		Mode   string `json:"mode" enum:"friends,studyBuddy,dating"`
		Action string `json:"action" enum:"like,pass"`
	}
}

type actOutput struct {
	Body actionResult
}

var actOperation = auth.Protected(huma.Operation{
	OperationID: "actOnCandidate",
	Method:      http.MethodPut,
	Path:        "/connections/candidates/{candidateId}",
	Summary:     "Like or pass a discovery card",
	Description: "A like cannot be undone. A pass hides the student for 30 days. Mutual likes in the same mode create one connection. A candidateId from another viewer returns 404.",
	Tags:        []string{"connections"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound, http.StatusTooManyRequests},
})

func (h *handler) act(ctx context.Context, in *actInput) (*actOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	result, err := h.connections.act(ctx, userID, in.CandidateID, in.Body.Mode, in.Body.Action)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &actOutput{Body: *result}, nil
}

type listInput struct {
	httpapi.PageQuery
}

type listOutput struct {
	Body struct {
		Items      []Connection `json:"items"`
		NextCursor *string      `json:"nextCursor" nullable:"true"`
	}
}

var listConnectionsOperation = auth.Protected(huma.Operation{
	OperationID: "listConnections",
	Method:      http.MethodGet,
	Path:        "/connections",
	Summary:     "List matches",
	Tags:        []string{"connections"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *handler) listConnections(ctx context.Context, in *listInput) (*listOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	items, next, err := h.connections.list(ctx, userID, in.Cursor, in.Limit)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &listOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

type deleteInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

var deleteConnectionOperation = auth.Protected(huma.Operation{
	OperationID: "deleteConnection",
	Method:      http.MethodDelete,
	Path:        "/connections/{id}",
	Summary:     "Remove a match for both students",
	Tags:        []string{"connections"},
	Errors:      []int{http.StatusNotFound},
})

func (h *handler) deleteConnection(ctx context.Context, in *deleteInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	if err := h.connections.repo.delete(ctx, userID, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return &struct{}{}, nil
}

type blockInput struct {
	ProfileID uuid.UUID `path:"profileId" format:"uuid"`
}

var blockOperation = auth.Protected(huma.Operation{
	OperationID: "blockStudent",
	Method:      http.MethodPost,
	Path:        "/blocks/{profileId}",
	Summary:     "Block a student",
	Description: "A block hides each student from the other's discovery, profile list and connections.",
	Tags:        []string{"connections"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *handler) block(ctx context.Context, in *blockInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	if err := h.connections.repo.block(ctx, userID, in.ProfileID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return &struct{}{}, nil
}

var unblockOperation = auth.Protected(huma.Operation{
	OperationID: "unblockStudent",
	Method:      http.MethodDelete,
	Path:        "/blocks/{profileId}",
	Summary:     "Remove a block",
	Tags:        []string{"connections"},
})

func (h *handler) unblock(ctx context.Context, in *blockInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	if err := h.connections.repo.unblock(ctx, userID, in.ProfileID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return &struct{}{}, nil
}
