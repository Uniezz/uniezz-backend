package feed

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type likeHandler struct {
	feed Module
}

type LikeState struct {
	Liked      bool `json:"liked"`
	LikesCount int  `json:"likesCount"`
}

type likeInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

type likeOutput struct {
	Body LikeState
}

type likePostOutput struct {
	Body struct {
		LikeState
		CoinsEarned int `json:"coinsEarned" minimum:"0" doc:"Uniezz coins this request paid you, daily quest reward included; 0 when it paid nothing" example:"1"`
	}
}

func likeOperation(id, method, path, summary, description string) huma.Operation {
	return auth.Protected(huma.Operation{
		OperationID: id,
		Method:      method,
		Path:        path,
		Summary:     summary,
		Description: description,
		Tags:        []string{"feed"},
		Errors:      []int{http.StatusNotFound},
	})
}

var (
	likePostOperation      = likeOperation("likeFeedPost", http.MethodPut, "/feed/posts/{id}/like", "Like a post", "Idempotent: repeating it changes nothing. The first like on another student's post pays you and the author 1 Uniezz coin each; unliking does not take them back.")
	unlikePostOperation    = likeOperation("unlikeFeedPost", http.MethodDelete, "/feed/posts/{id}/like", "Remove your like from a post", "Idempotent: repeating it changes nothing.")
	likeCommentOperation   = likeOperation("likeFeedComment", http.MethodPut, "/feed/comments/{id}/like", "Like a comment", "Idempotent: repeating it changes nothing.")
	unlikeCommentOperation = likeOperation("unlikeFeedComment", http.MethodDelete, "/feed/comments/{id}/like", "Remove your like from a comment", "Idempotent: repeating it changes nothing.")
)

func (h *likeHandler) likePost(ctx context.Context, in *likeInput) (*likePostOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	state, earned, err := h.feed.setPostLike(ctx, userID, in.ID, true)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &likePostOutput{}
	out.Body.LikeState = *state
	out.Body.CoinsEarned = earned
	return out, nil
}

func (h *likeHandler) unlikePost(ctx context.Context, in *likeInput) (*likeOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	state, _, err := h.feed.setPostLike(ctx, userID, in.ID, false)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &likeOutput{Body: *state}, nil
}

func (h *likeHandler) likeComment(ctx context.Context, in *likeInput) (*likeOutput, error) {
	return h.set(ctx, in.ID, true, h.feed.setCommentLike)
}

func (h *likeHandler) unlikeComment(ctx context.Context, in *likeInput) (*likeOutput, error) {
	return h.set(ctx, in.ID, false, h.feed.setCommentLike)
}

func (h *likeHandler) set(ctx context.Context, id uuid.UUID, liked bool, set func(context.Context, uuid.UUID, uuid.UUID, bool) (*LikeState, error)) (*likeOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	state, err := set(ctx, userID, id, liked)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &likeOutput{Body: *state}, nil
}
