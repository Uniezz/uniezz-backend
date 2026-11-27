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

type eventHandler struct {
	chat Module
}

type Event struct {
	Title      string       `json:"title" example:"Study session"`
	StartsAt   time.Time    `json:"startsAt"`
	EndsAt     *time.Time   `json:"endsAt" nullable:"true" doc:"Null when the event has no set end"`
	Place      *string      `json:"place" nullable:"true" example:"Library, floor 2" doc:"Null when no place was given"`
	CanceledAt *time.Time   `json:"canceledAt" nullable:"true" doc:"Set once the creator or the group owner canceled the event; null otherwise"`
	Ended      bool         `json:"ended" doc:"True after endsAt, or after startsAt when there is no end; answers are then refused"`
	Going      int          `json:"going"`
	Maybe      int          `json:"maybe"`
	No         int          `json:"no"`
	Attendees  []Student    `json:"attendees" doc:"Up to 5 students who answered going or maybe, latest first; empty in stream events, so fetch the message to show them"`
	MyAnswer   *EventAnswer `json:"myAnswer" nullable:"true" enum:"going,maybe,no" doc:"Null before the viewer answers and in stream events"`
}

type eventRequest struct {
	Title    string     `json:"title" minLength:"1" maxLength:"400" example:"Study session" doc:"Up to 100 characters after HTML is stripped"`
	StartsAt time.Time  `json:"startsAt" doc:"In the future"`
	EndsAt   *time.Time `json:"endsAt,omitempty" doc:"After startsAt"`
	Place    *string    `json:"place,omitempty" maxLength:"400" example:"Library, floor 2" doc:"Up to 100 characters after HTML is stripped"`
}

func (r *eventRequest) change() *eventChange {
	if r == nil {
		return nil
	}
	return &eventChange{Title: r.Title, StartsAt: r.StartsAt, EndsAt: r.EndsAt, Place: r.Place}
}

type answerChatEventInput struct {
	ID   uuid.UUID `path:"id" format:"uuid"`
	Body struct {
		Answer EventAnswer `json:"answer" enum:"going,maybe,no"`
	}
}

type answerChatEventOutput struct {
	Body Message
}

var answerChatEventOperation = auth.Authenticated(huma.Operation{
	OperationID: "answerChatEvent",
	Method:      http.MethodPut,
	Path:        "/chat/messages/{id}/rsvp",
	Summary:     "Answer an event message with going, maybe or no",
	Description: "Replaces the viewer's earlier answer; sending the same answer again changes nothing. A canceled event answers 409 event_canceled and an ended one 409 event_ended.",
	Tags:        []string{"chat"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict},
})

func (h *eventHandler) answer(ctx context.Context, in *answerChatEventInput) (*answerChatEventOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	message, err := h.chat.answerEvent(ctx, userID, answerChange{MessageID: in.ID, Answer: in.Body.Answer})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &answerChatEventOutput{Body: *message}, nil
}

type deleteMyChatEventAnswerInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

var deleteMyChatEventAnswerOperation = auth.Authenticated(huma.Operation{
	OperationID:   "deleteMyChatEventAnswer",
	Method:        http.MethodDelete,
	Path:          "/chat/messages/{id}/rsvp",
	Summary:       "Take back the viewer's answer to an event message",
	Description:   "Works on canceled and ended events too. Taking back a missing answer succeeds.",
	Tags:          []string{"chat"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusNotFound},
})

func (h *eventHandler) deleteAnswer(ctx context.Context, in *deleteMyChatEventAnswerInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	if err := h.chat.clearAnswer(ctx, userID, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}

type cancelChatEventInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

type cancelChatEventOutput struct {
	Body Message
}

var cancelChatEventOperation = auth.Authenticated(huma.Operation{
	OperationID: "cancelChatEvent",
	Method:      http.MethodPost,
	Path:        "/chat/messages/{id}/event/cancel",
	Summary:     "Cancel an event message",
	Description: "Only the event's creator or the group owner may cancel it, and a canceled event stays canceled. Canceling twice succeeds.",
	Tags:        []string{"chat"},
	Errors:      []int{http.StatusForbidden, http.StatusNotFound},
})

func (h *eventHandler) cancel(ctx context.Context, in *cancelChatEventInput) (*cancelChatEventOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	message, err := h.chat.cancelEvent(ctx, userID, in.ID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &cancelChatEventOutput{Body: *message}, nil
}
