package chat

import (
	"context"
	"net/http"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type locationHandler struct {
	chat Module
}

type LiveLocation struct {
	Until     time.Time  `json:"until" doc:"Sharing ends by itself at this time, one hour after it started"`
	StoppedAt *time.Time `json:"stoppedAt" nullable:"true" doc:"When sharing ended, by the sender or at until; null while it runs"`
	UpdatedAt time.Time  `json:"updatedAt" doc:"Time of the current position"`
	Accuracy  *int       `json:"accuracy,omitempty" doc:"Radius in meters; omitted when unknown or after sharing ended"`
	Heading   *int       `json:"heading,omitempty" doc:"Direction in degrees, 1 to 360; omitted when unknown or after sharing ended"`
}

type Location struct {
	Lat   float64       `json:"lat" example:"51.2465"`
	Lng   float64       `json:"lng" example:"22.5684"`
	Label *string       `json:"label" nullable:"true" example:"UMCS main gate" doc:"Null when no label was given"`
	Live  *LiveLocation `json:"live,omitempty" doc:"Set for a live location; lat and lng are then its latest position. Live positions arrive as location.updated stream events"`
}

type locationRequest struct {
	Lat   float64 `json:"lat" minimum:"-90" maximum:"90" example:"51.2465"`
	Lng   float64 `json:"lng" minimum:"-180" maximum:"180" example:"22.5684"`
	Label *string `json:"label,omitempty" maxLength:"400" example:"UMCS main gate" doc:"Up to 100 characters after HTML is stripped"`
	Live  bool    `json:"live,omitempty" doc:"Share the live position for one hour with updateChatLocation"`
}

func (r *locationRequest) change() *locationChange {
	if r == nil {
		return nil
	}
	return &locationChange{Lat: r.Lat, Lng: r.Lng, Label: r.Label, Live: r.Live}
}

type updateChatLocationInput struct {
	ID   uuid.UUID `path:"id" format:"uuid"`
	Body struct {
		Lat      float64 `json:"lat" minimum:"-90" maximum:"90" example:"51.2465"`
		Lng      float64 `json:"lng" minimum:"-180" maximum:"180" example:"22.5684"`
		Accuracy *int    `json:"accuracy,omitempty" minimum:"0" maximum:"1500" doc:"Radius in meters"`
		Heading  *int    `json:"heading,omitempty" minimum:"1" maximum:"360" doc:"Direction in degrees"`
	}
}

var updateChatLocationOperation = auth.Authenticated(huma.Operation{
	OperationID:   "updateChatLocation",
	Method:        http.MethodPost,
	Path:          "/chat/messages/{id}/location",
	Summary:       "Send a new position for the sender's live location message",
	Description:   "At most one update per 10 seconds (429 rate_limited); only the latest position is kept. Members get it as a location.updated stream event, not as message.updated. After sharing ended the answer is 409 location_not_live.",
	Tags:          []string{"chat"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests},
})

func (h *locationHandler) update(ctx context.Context, in *updateChatLocationInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	err := h.chat.updateLocation(ctx, userID, positionChange{
		MessageID: in.ID,
		Lat:       in.Body.Lat,
		Lng:       in.Body.Lng,
		Accuracy:  in.Body.Accuracy,
		Heading:   in.Body.Heading,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}

type stopChatLocationInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

type stopChatLocationOutput struct {
	Body Message
}

var stopChatLocationOperation = auth.Authenticated(huma.Operation{
	OperationID: "stopChatLocation",
	Method:      http.MethodPost,
	Path:        "/chat/messages/{id}/location/stop",
	Summary:     "Stop sharing the sender's live location",
	Description: "The message keeps the last position. Stopping a location that is not live any more succeeds.",
	Tags:        []string{"chat"},
	Errors:      []int{http.StatusForbidden, http.StatusNotFound},
})

func (h *locationHandler) stop(ctx context.Context, in *stopChatLocationInput) (*stopChatLocationOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	message, err := h.chat.stopLocation(ctx, userID, in.ID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &stopChatLocationOutput{Body: *message}, nil
}
