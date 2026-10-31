package feed

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
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

func likeOperation(id, method, path, summary string) huma.Operation {
	return auth.Protected(huma.Operation{
		OperationID: id,
		Method:      method,
		Path:        path,
		Summary:     summary,
		Description: "Idempotent: repeating it changes nothing.",
		Tags:        []string{"feed"},
		Errors:      []int{http.StatusNotFound},
	})
}

var (
	likePostOperation      = likeOperation("likeFeedPost", http.MethodPut, "/feed/posts/{id}/like", "Like a post")
	unlikePostOperation    = likeOperation("unlikeFeedPost", http.MethodDelete, "/feed/posts/{id}/like", "Remove your like from a post")
	likeCommentOperation   = likeOperation("likeFeedComment", http.MethodPut, "/feed/comments/{id}/like", "Like a comment")
	unlikeCommentOperation = likeOperation("unlikeFeedComment", http.MethodDelete, "/feed/comments/{id}/like", "Remove your like from a comment")
)

func (h *likeHandler) likePost(ctx context.Context, in *likeInput) (*likeOutput, error) {
	return h.set(ctx, in.ID, true, h.feed.setPostLike)
}

func (h *likeHandler) unlikePost(ctx context.Context, in *likeInput) (*likeOutput, error) {
	return h.set(ctx, in.ID, false, h.feed.setPostLike)
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
		return nil, unauthorized()
	}

	state, err := set(ctx, userID, id, liked)
	if err != nil {
		return nil, apiError(err)
	}
	return &likeOutput{Body: *state}, nil
}
