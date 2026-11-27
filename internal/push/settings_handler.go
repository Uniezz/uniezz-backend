package push

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
)

type settingsHandler struct {
	push Module
}

type NotificationSettings struct {
	Enabled bool `json:"enabled" doc:"false stops every push notification to the student's devices"`
}

type getMyPushSettingsOutput struct {
	Body NotificationSettings
}

var getMyPushSettingsOperation = auth.Authenticated(huma.Operation{
	OperationID: "getMyPushSettings",
	Method:      http.MethodGet,
	Path:        "/push/settings/me",
	Summary:     "Get the logged-in student's push notification settings",
	Tags:        []string{"push"},
})

func (h *settingsHandler) getSettings(ctx context.Context, _ *struct{}) (*getMyPushSettingsOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	settings, err := h.push.getSettings(ctx, userID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &getMyPushSettingsOutput{Body: *settings}, nil
}

type updateMyPushSettingsInput struct {
	Body struct {
		Enabled *bool `json:"enabled,omitempty" doc:"false stops every push notification to the student's devices"`
	}
}

type updateMyPushSettingsOutput struct {
	Body NotificationSettings
}

var updateMyPushSettingsOperation = auth.Authenticated(huma.Operation{
	OperationID: "updateMyPushSettings",
	Method:      http.MethodPatch,
	Path:        "/push/settings/me",
	Summary:     "Update the logged-in student's push notification settings",
	Description: "Fields left out keep their value.",
	Tags:        []string{"push"},
})

func (h *settingsHandler) updateSettings(ctx context.Context, in *updateMyPushSettingsInput) (*updateMyPushSettingsOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	settings, err := h.push.updateSettings(ctx, userID, settingsChange{Enabled: in.Body.Enabled})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &updateMyPushSettingsOutput{Body: *settings}, nil
}
