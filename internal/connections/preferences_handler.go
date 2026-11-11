package connections

import (
	"context"
	"net/http"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
)

type preferencesHandler struct {
	connections Module
}

type savePreferencesInput struct {
	Body struct {
		BirthDate string   `json:"birthDate,omitempty" format:"date" doc:"YYYY-MM-DD. Age must be 16 to 100."`
		OpenTo    []string `json:"openTo,omitempty" enum:"friends,studyBuddy,dating" doc:"Who the student can discover and be discovered as. dating needs an age of at least 18."`
		Intent    *string  `json:"intent,omitempty" maxLength:"200" doc:"Shown on the student's discovery card. An empty string clears it."`
	}
}

var savePreferencesOperation = auth.Protected(huma.Operation{
	OperationID: "saveConnectionPreferences",
	Method:      http.MethodPatch,
	Path:        "/connections/me",
	Summary:     "Set what Connections may show",
	Description: "A missing field keeps its current value. Birth date is used only to compute age.",
	Tags:        []string{"connections"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *preferencesHandler) savePreferences(ctx context.Context, in *savePreferencesInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	change := preferencesChange{OpenTo: in.Body.OpenTo, SetIntent: in.Body.Intent != nil, Intent: in.Body.Intent}
	if in.Body.BirthDate != "" {
		birthDate, err := time.Parse(time.DateOnly, in.Body.BirthDate)
		if err != nil {
			return nil, errorResponses.Error(ErrInvalidInput)
		}
		change.BirthDate = &birthDate
	}

	if err := h.connections.savePreferences(ctx, userID, change); err != nil {
		return nil, errorResponses.Error(err)
	}
	return &struct{}{}, nil
}
