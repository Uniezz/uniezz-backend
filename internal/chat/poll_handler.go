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

type pollHandler struct {
	chat Module
}

type PollOption struct {
	ID    int    `json:"id" minimum:"1" maximum:"10"`
	Text  string `json:"text" example:"Library"`
	Votes int    `json:"votes" doc:"Students who chose this option"`
}

type Poll struct {
	Question       string       `json:"question" example:"Where do we study on Friday?"`
	Options        []PollOption `json:"options"`
	MultipleChoice bool         `json:"multipleChoice"`
	Anonymous      bool         `json:"anonymous" doc:"When true, voters are never listed"`
	ClosesAt       *time.Time   `json:"closesAt" nullable:"true" doc:"When the poll closes by itself; null when it stays open until closed"`
	ClosedAt       *time.Time   `json:"closedAt" nullable:"true" doc:"When the creator or the group owner closed it; null otherwise"`
	Closed         bool         `json:"closed" doc:"True once closedAt is set or closesAt has passed; votes are then refused"`
	TotalVoters    int          `json:"totalVoters"`
	Voters         []Student    `json:"voters" doc:"Up to 5 latest voters of a public poll; empty for anonymous polls and in stream events, so fetch the message to show them"`
	MyVote         []int        `json:"myVote" doc:"Option ids the viewer chose; empty before voting and in stream events, so keep the value from your own vote"`
}

type pollRequest struct {
	Question string `json:"question" minLength:"1" maxLength:"1200" example:"Where do we study on Friday?" doc:"Up to 300 characters after HTML is stripped"`
	Options  []struct {
		Text string `json:"text" minLength:"1" maxLength:"400" example:"Library" doc:"Up to 100 characters after HTML is stripped"`
	} `json:"options" minItems:"2" maxItems:"10" doc:"Options get ids 1 to n in this order"`
	MultipleChoice bool       `json:"multipleChoice,omitempty"`
	Anonymous      bool       `json:"anonymous,omitempty" doc:"Hide who voted for what"`
	ClosesAt       *time.Time `json:"closesAt,omitempty" doc:"Closes the poll at this time, at most 30 days ahead"`
}

func (r *pollRequest) change() *pollChange {
	if r == nil {
		return nil
	}
	c := &pollChange{Question: r.Question, MultipleChoice: r.MultipleChoice, Anonymous: r.Anonymous, ClosesAt: r.ClosesAt}
	for _, o := range r.Options {
		c.Options = append(c.Options, o.Text)
	}
	return c
}

type voteChatPollInput struct {
	ID   uuid.UUID `path:"id" format:"uuid"`
	Body struct {
		OptionIDs []int `json:"optionIds" maxItems:"10" uniqueItems:"true" doc:"The viewer's whole choice, replacing an earlier vote; one id unless the poll is multipleChoice. An empty list takes the vote back"`
	}
}

type voteChatPollOutput struct {
	Body Message
}

var voteChatPollOperation = auth.Authenticated(huma.Operation{
	OperationID: "voteChatPoll",
	Method:      http.MethodPut,
	Path:        "/chat/messages/{id}/poll/vote",
	Summary:     "Vote in a poll message, change the vote or take it back",
	Description: "Sending the current choice again changes nothing. A closed poll refuses votes with 409 poll_closed.",
	Tags:        []string{"chat"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict},
})

func (h *pollHandler) vote(ctx context.Context, in *voteChatPollInput) (*voteChatPollOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	message, err := h.chat.votePoll(ctx, userID, voteChange{MessageID: in.ID, OptionIDs: in.Body.OptionIDs})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &voteChatPollOutput{Body: *message}, nil
}

type closeChatPollInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

type closeChatPollOutput struct {
	Body Message
}

var closeChatPollOperation = auth.Authenticated(huma.Operation{
	OperationID: "closeChatPoll",
	Method:      http.MethodPost,
	Path:        "/chat/messages/{id}/poll/close",
	Summary:     "Close a poll message",
	Description: "Only the poll's creator or the group owner may close it. Closing a closed poll succeeds.",
	Tags:        []string{"chat"},
	Errors:      []int{http.StatusForbidden, http.StatusNotFound},
})

func (h *pollHandler) close(ctx context.Context, in *closeChatPollInput) (*closeChatPollOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	message, err := h.chat.closePoll(ctx, userID, in.ID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &closeChatPollOutput{Body: *message}, nil
}
