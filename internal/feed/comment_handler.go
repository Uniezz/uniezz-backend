package feed

import (
	"context"
	"net/http"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type commentHandler struct {
	feed Module
}

type Comment struct {
	ID               uuid.UUID  `json:"id" format:"uuid"`
	PostID           uuid.UUID  `json:"postId" format:"uuid"`
	ParentID         *uuid.UUID `json:"parentId" nullable:"true" doc:"Root comment of the thread; null for a root comment"`
	ReplyToCommentID *uuid.UUID `json:"replyToCommentId" nullable:"true" doc:"Comment this reply answers; null for a root comment or when that comment was deleted"`
	Content          *string    `json:"content" nullable:"true" doc:"Null when deleted"`
	Deleted          bool       `json:"deleted" doc:"A deleted comment is listed only while it still has replies"`
	Author           *Author    `json:"author,omitempty" doc:"Missing when deleted"`
	LikesCount       int        `json:"likesCount"`
	RepliesCount     int        `json:"repliesCount"`
	LikedByMe        bool       `json:"likedByMe"`
	CreatedAt        time.Time  `json:"createdAt"`
	EditedAt         *time.Time `json:"editedAt" nullable:"true"`
}

type createCommentInput struct {
	ID             uuid.UUID `path:"id" format:"uuid"`
	IdempotencyKey string    `header:"Idempotency-Key" minLength:"1" maxLength:"255" doc:"Repeating a request with the same key returns the comment the first one created instead of creating another. Keys are kept for at least 24 hours."`
	Body           struct {
		Content          string     `json:"content" maxLength:"4000" doc:"1–1000 characters after HTML is stripped"`
		ReplyToCommentID *uuid.UUID `json:"replyToCommentId,omitempty" doc:"Comment to reply to; a reply to a reply joins the same thread"`
	}
}

type createCommentOutput struct {
	Replayed string `header:"Idempotent-Replayed" doc:"true when the comment was created by an earlier request with the same key"`
	Body     Comment
}

var createCommentOperation = auth.Protected(huma.Operation{
	OperationID:   "createFeedComment",
	Method:        http.MethodPost,
	Path:          "/feed/posts/{id}/comments",
	Summary:       "Comment on a post or reply to a comment",
	Tags:          []string{"feed"},
	DefaultStatus: http.StatusCreated,
	Errors:        []int{http.StatusBadRequest, http.StatusNotFound, http.StatusGone, http.StatusUnprocessableEntity},
})

func (h *commentHandler) createComment(ctx context.Context, in *createCommentInput) (*createCommentOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	comment, replayed, err := h.feed.createComment(ctx, userID, commentInput{
		PostID:           in.ID,
		ReplyToCommentID: in.Body.ReplyToCommentID,
		Content:          in.Body.Content,
	}, in.IdempotencyKey)
	if err != nil {
		return nil, apiError(err)
	}

	out := &createCommentOutput{Body: *comment}
	if replayed {
		out.Replayed = "true"
	}
	return out, nil
}

type listCommentsInput struct {
	httpapi.PageQuery
	ID uuid.UUID `path:"id" format:"uuid"`
}

type listCommentsOutput struct {
	Body struct {
		Items      []Comment `json:"items"`
		NextCursor *string   `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get the next page; null on the last page"`
	}
}

var listCommentsOperation = auth.Protected(huma.Operation{
	OperationID: "listFeedComments",
	Method:      http.MethodGet,
	Path:        "/feed/posts/{id}/comments",
	Summary:     "List root comments of a post",
	Description: "Oldest first. Replies are listed per root comment.",
	Tags:        []string{"feed"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *commentHandler) listComments(ctx context.Context, in *listCommentsInput) (*listCommentsOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	items, next, err := h.feed.listComments(ctx, userID, in.ID, in.Cursor, in.Limit)
	if err != nil {
		return nil, apiError(err)
	}
	out := &listCommentsOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

var listRepliesOperation = auth.Protected(huma.Operation{
	OperationID: "listFeedCommentReplies",
	Method:      http.MethodGet,
	Path:        "/feed/comments/{id}/replies",
	Summary:     "List replies to a root comment",
	Description: "Oldest first.",
	Tags:        []string{"feed"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *commentHandler) listReplies(ctx context.Context, in *listCommentsInput) (*listCommentsOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	items, next, err := h.feed.listReplies(ctx, userID, in.ID, in.Cursor, in.Limit)
	if err != nil {
		return nil, apiError(err)
	}
	out := &listCommentsOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

type updateCommentInput struct {
	ID   uuid.UUID `path:"id" format:"uuid"`
	Body struct {
		Content string `json:"content" maxLength:"4000"`
	}
}

type commentOutput struct {
	Body Comment
}

var updateCommentOperation = auth.Protected(huma.Operation{
	OperationID: "updateFeedComment",
	Method:      http.MethodPatch,
	Path:        "/feed/comments/{id}",
	Summary:     "Edit your comment",
	Tags:        []string{"feed"},
	Errors:      []int{http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound},
})

func (h *commentHandler) updateComment(ctx context.Context, in *updateCommentInput) (*commentOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	comment, err := h.feed.updateComment(ctx, userID, in.ID, in.Body.Content)
	if err != nil {
		return nil, apiError(err)
	}
	return &commentOutput{Body: *comment}, nil
}

type commentPath struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

var deleteCommentOperation = auth.Protected(huma.Operation{
	OperationID:   "deleteFeedComment",
	Method:        http.MethodDelete,
	Path:          "/feed/comments/{id}",
	Summary:       "Delete your comment",
	Description:   "A root comment with replies stays in the thread as deleted, without content or author.",
	Tags:          []string{"feed"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusForbidden, http.StatusNotFound},
})

func (h *commentHandler) deleteComment(ctx context.Context, in *commentPath) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	if err := h.feed.deleteComment(ctx, userID, in.ID); err != nil {
		return nil, apiError(err)
	}
	return nil, nil
}
