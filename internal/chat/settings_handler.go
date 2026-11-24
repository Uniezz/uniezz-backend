package chat

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
)

type settingsHandler struct {
	chat Module
}

type WhoCanMessage string

const (
	WhoEveryone   WhoCanMessage = "everyone"
	WhoUniversity WhoCanMessage = "university"
	WhoMatches    WhoCanMessage = "matches"
)

type Settings struct {
	WhoCanMessage WhoCanMessage `json:"whoCanMessage" enum:"everyone,university,matches" doc:"Who may send the student a message request: everyone, students of the same university, or only Connections"`
}

type getMyChatSettingsOutput struct {
	Body Settings
}

var getMyChatSettingsOperation = auth.Authenticated(huma.Operation{
	OperationID: "getMyChatSettings",
	Method:      http.MethodGet,
	Path:        "/chat/settings/me",
	Summary:     "Get the logged-in student's chat settings",
	Tags:        []string{"chat"},
})

func (h *settingsHandler) getSettings(ctx context.Context, _ *struct{}) (*getMyChatSettingsOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	settings, err := h.chat.getSettings(ctx, userID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &getMyChatSettingsOutput{Body: *settings}, nil
}

type updateMyChatSettingsInput struct {
	Body struct {
		WhoCanMessage *WhoCanMessage `json:"whoCanMessage,omitempty" enum:"everyone,university,matches" doc:"Changes only new message requests; existing conversations stay"`
	}
}

type updateMyChatSettingsOutput struct {
	Body Settings
}

var updateMyChatSettingsOperation = auth.Authenticated(huma.Operation{
	OperationID: "updateMyChatSettings",
	Method:      http.MethodPatch,
	Path:        "/chat/settings/me",
	Summary:     "Update the logged-in student's chat settings",
	Description: "Fields left out keep their value.",
	Tags:        []string{"chat"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *settingsHandler) updateSettings(ctx context.Context, in *updateMyChatSettingsInput) (*updateMyChatSettingsOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	settings, err := h.chat.updateSettings(ctx, userID, settingsChange{WhoCanMessage: in.Body.WhoCanMessage})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &updateMyChatSettingsOutput{Body: *settings}, nil
}
