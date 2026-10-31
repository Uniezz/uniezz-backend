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

type postHandler struct {
	feed Module
}

type Author struct {
	ID           uuid.UUID `json:"id" format:"uuid"`
	FirstName    *string   `json:"firstName" nullable:"true" example:"Jan"`
	LastName     *string   `json:"lastName" nullable:"true" example:"Kowalski"`
	UniversityID string    `json:"universityId" example:"umcs"`
	AvatarURL    *string   `json:"avatarUrl" nullable:"true"`
}

type Post struct {
	ID            uuid.UUID  `json:"id" format:"uuid"`
	Type          string     `json:"type" enum:"announcement,event,question"`
	University    string     `json:"university" example:"umcs" doc:"Target university, or all"`
	Title         string     `json:"title"`
	Content       string     `json:"content"`
	Tags          []string   `json:"tags"`
	EventDate     *time.Time `json:"eventDate" nullable:"true"`
	MediaURLs     []string   `json:"mediaUrls"`
	Author        Author     `json:"author"`
	LikesCount    int        `json:"likesCount"`
	CommentsCount int        `json:"commentsCount"`
	LikedByMe     bool       `json:"likedByMe"`
	CreatedAt     time.Time  `json:"createdAt"`
	EditedAt      *time.Time `json:"editedAt" nullable:"true"`
}

type createPostInput struct {
	IdempotencyKey string `header:"Idempotency-Key" minLength:"1" maxLength:"255" doc:"Repeating a request with the same key returns the resource the first one created instead of creating another. Keys are kept for at least 24 hours."`
	Body           struct {
		Type       string      `json:"type" enum:"announcement,event,question"`
		University string      `json:"university" maxLength:"32" doc:"Your own university, or all" example:"umcs"`
		Title      string      `json:"title" maxLength:"600" doc:"3–150 characters after HTML is stripped"`
		Content    string      `json:"content" maxLength:"20000" doc:"10–5000 characters after HTML is stripped"`
		Tags       []string    `json:"tags,omitempty" maxItems:"20" doc:"Up to 10 tags of up to 30 characters"`
		EventDate  *time.Time  `json:"eventDate,omitempty" doc:"Only for event posts"`
		MediaIDs   []uuid.UUID `json:"mediaIds,omitempty" maxItems:"10" uniqueItems:"true" doc:"Uploads of kind feed, shown in this order"`
	}
}

type createPostOutput struct {
	Replayed string `header:"Idempotent-Replayed" doc:"true when the post was created by an earlier request with the same key"`
	Body     Post
}

var createPostOperation = auth.Protected(huma.Operation{
	OperationID:   "createFeedPost",
	Method:        http.MethodPost,
	Path:          "/feed/posts",
	Summary:       "Create a feed post",
	Tags:          []string{"feed"},
	DefaultStatus: http.StatusCreated,
	Errors:        []int{http.StatusBadRequest, http.StatusGone, http.StatusUnprocessableEntity},
})

func (h *postHandler) createPost(ctx context.Context, in *createPostInput) (*createPostOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	post, replayed, err := h.feed.createPost(ctx, userID, postInput{
		Type:       in.Body.Type,
		University: in.Body.University,
		Title:      in.Body.Title,
		Content:    in.Body.Content,
		Tags:       in.Body.Tags,
		EventDate:  in.Body.EventDate,
		MediaIDs:   in.Body.MediaIDs,
	}, in.IdempotencyKey)
	if err != nil {
		return nil, apiError(err)
	}

	out := &createPostOutput{Body: *post}
	if replayed {
		out.Replayed = "true"
	}
	return out, nil
}

type listPostsInput struct {
	httpapi.PageQuery
	University string    `query:"university" maxLength:"32" doc:"Posts targeted to this university or to all; all alone returns only posts targeted to everyone" example:"umcs"`
	Type       string    `query:"type" enum:"announcement,event,question"`
	From       time.Time `query:"from" doc:"Created at or after this time"`
	To         time.Time `query:"to" doc:"Created before this time"`
}

type listPostsOutput struct {
	Body struct {
		Items      []Post  `json:"items"`
		NextCursor *string `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get the next page; null on the last page"`
	}
}

var listPostsOperation = auth.Protected(huma.Operation{
	OperationID: "listFeedPosts",
	Method:      http.MethodGet,
	Path:        "/feed/posts",
	Summary:     "List feed posts",
	Description: "Newest first, paginated by cursor.",
	Tags:        []string{"feed"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *postHandler) listPosts(ctx context.Context, in *listPostsInput) (*listPostsOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	items, next, err := h.feed.listPosts(ctx, postQuery{
		ViewerID:   userID,
		University: in.University,
		Type:       in.Type,
		From:       optionalTime(in.From),
		To:         optionalTime(in.To),
		Cursor:     in.Cursor,
		Limit:      in.Limit,
	})
	if err != nil {
		return nil, apiError(err)
	}

	out := &listPostsOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

type postPath struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

type postOutput struct {
	Body Post
}

var getPostOperation = auth.Protected(huma.Operation{
	OperationID: "getFeedPost",
	Method:      http.MethodGet,
	Path:        "/feed/posts/{id}",
	Summary:     "Get a feed post",
	Tags:        []string{"feed"},
	Errors:      []int{http.StatusNotFound},
})

func (h *postHandler) getPost(ctx context.Context, in *postPath) (*postOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	post, err := h.feed.post(ctx, userID, in.ID)
	if err != nil {
		return nil, apiError(err)
	}
	return &postOutput{Body: *post}, nil
}

type updatePostInput struct {
	ID   uuid.UUID `path:"id" format:"uuid"`
	Body struct {
		Title     *string    `json:"title,omitempty" maxLength:"600"`
		Content   *string    `json:"content,omitempty" maxLength:"20000"`
		EventDate *time.Time `json:"eventDate,omitempty" doc:"Only for event posts"`
		Tags      *[]string  `json:"tags,omitempty" maxItems:"20"`
	}
}

var updatePostOperation = auth.Protected(huma.Operation{
	OperationID: "updateFeedPost",
	Method:      http.MethodPatch,
	Path:        "/feed/posts/{id}",
	Summary:     "Edit your feed post",
	Description: "Only fields that are sent change. Type, target university and media cannot change.",
	Tags:        []string{"feed"},
	Errors:      []int{http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound},
})

func (h *postHandler) updatePost(ctx context.Context, in *updatePostInput) (*postOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	post, err := h.feed.updatePost(ctx, userID, in.ID, postEdit{
		Title:     in.Body.Title,
		Content:   in.Body.Content,
		EventDate: in.Body.EventDate,
		Tags:      in.Body.Tags,
	})
	if err != nil {
		return nil, apiError(err)
	}
	return &postOutput{Body: *post}, nil
}

var deletePostOperation = auth.Protected(huma.Operation{
	OperationID:   "deleteFeedPost",
	Method:        http.MethodDelete,
	Path:          "/feed/posts/{id}",
	Summary:       "Delete your feed post",
	Description:   "Deletes the post with its comments, likes and media.",
	Tags:          []string{"feed"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusForbidden, http.StatusNotFound},
})

func (h *postHandler) deletePost(ctx context.Context, in *postPath) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	if err := h.feed.deletePost(ctx, userID, in.ID); err != nil {
		return nil, apiError(err)
	}
	return nil, nil
}
