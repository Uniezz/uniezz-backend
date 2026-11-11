package connections

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
)

type preferencesHandler struct {
	connections Module
}

type Preferences struct {
	BirthDate *string `json:"birthDate" nullable:"true" format:"date" doc:"Null until set" example:"2003-05-17"`
	OpenTo    []Mode  `json:"openTo" enum:"friends,studyBuddy,dating" doc:"Who the student can discover and be discovered as"`
	Intent    *string `json:"intent" nullable:"true" doc:"Shown on the student's discovery card. Null when unset." example:"Looking for a climbing partner"`
}

type getPreferencesOutput struct {
	Body Preferences
}

var getPreferencesOperation = auth.Protected(huma.Operation{
	OperationID: "getMyConnectionPreferences",
	Method:      http.MethodGet,
	Path:        "/connections/me",
	Summary:     "Get what Connections may show about the logged-in student",
	Tags:        []string{"connections"},
})

func (h *preferencesHandler) getPreferences(ctx context.Context, _ *struct{}) (*getPreferencesOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	p, err := h.connections.getPreferences(ctx, userID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &getPreferencesOutput{Body: *p}, nil
}

type savePreferencesInput struct {
	Body struct {
		BirthDate *string `json:"birthDate,omitempty" format:"date" doc:"Age must be 16 to 100" example:"2003-05-17"`
		OpenTo    []Mode  `json:"openTo,omitempty" enum:"friends,studyBuddy,dating" maxItems:"3" doc:"dating needs an age of at least 18, else returns 400 dating_requires_adult"`
		Intent    *string `json:"intent,omitempty" maxLength:"1000" doc:"Up to 200 characters after HTML is stripped. An empty string clears it."`
	}
}

type savePreferencesOutput struct {
	Body Preferences
}

var savePreferencesOperation = auth.Protected(huma.Operation{
	OperationID: "updateMyConnectionPreferences",
	Method:      http.MethodPatch,
	Path:        "/connections/me",
	Summary:     "Set what Connections may show about the logged-in student",
	Description: "A missing field keeps its current value. Birth date is used only to compute age.",
	Tags:        []string{"connections"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *preferencesHandler) savePreferences(ctx context.Context, in *savePreferencesInput) (*savePreferencesOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	p, err := h.connections.savePreferences(ctx, userID, preferencesChange{
		BirthDate: in.Body.BirthDate,
		OpenTo:    in.Body.OpenTo,
		Intent:    in.Body.Intent,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &savePreferencesOutput{Body: *p}, nil
}
