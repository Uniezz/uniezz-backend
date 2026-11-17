package rewards

import (
	"context"
	"net/http"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
)

type grantHandler struct {
	rewards Module
}

func (Reason) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	schema.Enum = nil
	if schema.Extensions == nil {
		schema.Extensions = map[string]any{}
	}
	schema.Extensions["x-extensible-enum"] = []any{"welcome", "like_given", "like_received", "comment", "profile_complete", "birth_date", "referral", "referral_joined", "daily_quests", "faq_approved"}
	return schema
}

type CoinGrant struct {
	Reason    Reason    `json:"reason" doc:"What earned the coins. welcome: the new account. like_given: liking another student's post. like_received: another student liked your post. comment: the first comment or reply on another student's post. profile_complete: filling every profile field. birth_date: the first birth date saved in connections. referral: inviting another student who signs up. referral_joined: signing up with an invitation code. daily_quests: finishing every daily quest. faq_approved: a moderator approved your FAQ proposal. More values may be added; show a generic label for unknown ones."`
	Amount    int       `json:"amount" minimum:"1" example:"10"`
	CreatedAt time.Time `json:"createdAt"`
}

type listMyCoinGrantsInput struct {
	httpapi.PageQuery
}

type listMyCoinGrantsOutput struct {
	Body struct {
		Items      []CoinGrant `json:"items"`
		NextCursor *string     `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get the next page; null on the last page"`
	}
}

var listMyCoinGrantsOperation = auth.Authenticated(huma.Operation{
	OperationID: "listMyCoinGrants",
	Method:      http.MethodGet,
	Path:        "/rewards/me/grants",
	Summary:     "List the coins the logged-in student has earned",
	Description: "Newest first, paginated by cursor. Coins are only earned; nothing spends them yet.",
	Tags:        []string{"rewards"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *grantHandler) listMyCoinGrants(ctx context.Context, in *listMyCoinGrantsInput) (*listMyCoinGrantsOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	items, next, err := h.rewards.grants(ctx, userID, in.Cursor, in.Limit)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &listMyCoinGrantsOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}
