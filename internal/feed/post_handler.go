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
	SavedByMe     bool       `json:"savedByMe"`
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
		EventDate  *time.Time  `json:"eventDate,omitempty" doc:"Required for event posts, not allowed for others"`
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
		return nil, httpapi.Unauthorized()
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
		return nil, errorResponses.Error(err)
	}

	out := &createPostOutput{Body: *post}
	if replayed {
		out.Replayed = "true"
	}
	return out, nil
}

type PostFilters struct {
	Search     string    `query:"q" maxLength:"200" doc:"Up to 5 words of at least 3 characters. Every word must match the title, content, author name or target university ID or name, ignoring case and accents; blank means no search" example:"sesja umcs"`
	AuthorID   string    `query:"authorId" format:"uuid" doc:"Only posts by this user"`
	University string    `query:"university" maxLength:"32" doc:"Posts targeted to this university or to all; all alone returns only posts targeted to everyone" example:"umcs"`
	Type       string    `query:"type" enum:"announcement,event,question"`
	From       time.Time `query:"from" doc:"Created at or after this time"`
	To         time.Time `query:"to" doc:"Created before this time"`
}

func (f PostFilters) query(viewerID uuid.UUID) postQuery {
	return postQuery{
		Search:     f.Search,
		ViewerID:   viewerID,
		AuthorID:   f.AuthorID,
		University: f.University,
		Type:       f.Type,
		From:       optionalTime(f.From),
		To:         optionalTime(f.To),
	}
}

type listPostsInput struct {
	httpapi.PageQuery
	PostFilters
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
	Description: "Newest first by createdAt, then ID descending. All signed-in students can browse posts across universities. Keep the same search and filters when following a cursor; results reflect current data.",
	Tags:        []string{"feed"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *postHandler) listPosts(ctx context.Context, in *listPostsInput) (*listPostsOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	q := in.query(userID)
	q.Cursor, q.Limit = in.Cursor, in.Limit
	items, next, err := h.feed.listPosts(ctx, q)
	if err != nil {
		return nil, errorResponses.Error(err)
	}

	out := &listPostsOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

var (
	listSavedPostsOperation = auth.Protected(huma.Operation{
		OperationID: "listSavedFeedPosts",
		Method:      http.MethodGet,
		Path:        "/feed/posts/saved",
		Summary:     "List posts saved by the signed-in student",
		Description: "The caller's saves only, newest save first, then post ID descending. Each item is the same post as in the feed, including likedByMe and savedByMe. A deleted post is absent. Keep the same limit when following a cursor.",
		Tags:        []string{"feed"},
		Errors:      []int{http.StatusBadRequest},
	})
	listLikedPostsOperation = auth.Protected(huma.Operation{
		OperationID: "listLikedFeedPosts",
		Method:      http.MethodGet,
		Path:        "/feed/posts/liked",
		Summary:     "List posts liked by the signed-in student",
		Description: "The caller's likes only, newest like first, then post ID descending. Each item is the same post as in the feed, including likedByMe and savedByMe. A deleted post, or a like that was removed, is absent. Keep the same limit when following a cursor.",
		Tags:        []string{"feed"},
		Errors:      []int{http.StatusBadRequest},
	})
)

type activityPostsInput struct {
	httpapi.PageQuery
}

func (h *postHandler) listSavedPosts(ctx context.Context, in *activityPostsInput) (*listPostsOutput, error) {
	return h.listActivity(ctx, in, h.feed.listSavedPosts)
}

func (h *postHandler) listLikedPosts(ctx context.Context, in *activityPostsInput) (*listPostsOutput, error) {
	return h.listActivity(ctx, in, h.feed.listLikedPosts)
}

func (h *postHandler) listActivity(ctx context.Context, in *activityPostsInput, list func(context.Context, uuid.UUID, string, int) ([]Post, *string, error)) (*listPostsOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	items, next, err := list(ctx, userID, in.Cursor, in.Limit)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &listPostsOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

type saveState struct {
	Saved bool `json:"saved"`
}

type saveOutput struct {
	Body saveState
}

var (
	savePostOperation = auth.Protected(huma.Operation{
		OperationID: "saveFeedPost",
		Method:      http.MethodPut,
		Path:        "/feed/posts/{id}/save",
		Summary:     "Save a post",
		Description: "Idempotent: saving a post again changes nothing. A missing post returns 404.",
		Tags:        []string{"feed"},
		Errors:      []int{http.StatusNotFound},
	})
	unsavePostOperation = auth.Protected(huma.Operation{
		OperationID: "unsaveFeedPost",
		Method:      http.MethodDelete,
		Path:        "/feed/posts/{id}/save",
		Summary:     "Remove a saved post",
		Description: "Idempotent: removing a save that is not there changes nothing. A missing post returns 404.",
		Tags:        []string{"feed"},
		Errors:      []int{http.StatusNotFound},
	})
)

func (h *postHandler) savePost(ctx context.Context, in *likeInput) (*saveOutput, error) {
	return h.setSave(ctx, in.ID, true)
}

func (h *postHandler) unsavePost(ctx context.Context, in *likeInput) (*saveOutput, error) {
	return h.setSave(ctx, in.ID, false)
}

func (h *postHandler) setSave(ctx context.Context, postID uuid.UUID, saved bool) (*saveOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	if err := h.feed.setPostSave(ctx, userID, postID, saved); err != nil {
		return nil, errorResponses.Error(err)
	}
	return &saveOutput{Body: saveState{Saved: saved}}, nil
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
		return nil, httpapi.Unauthorized()
	}

	post, err := h.feed.post(ctx, userID, in.ID)
	if err != nil {
		return nil, errorResponses.Error(err)
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
		return nil, httpapi.Unauthorized()
	}

	post, err := h.feed.updatePost(ctx, userID, in.ID, postEdit{
		Title:     in.Body.Title,
		Content:   in.Body.Content,
		EventDate: in.Body.EventDate,
		Tags:      in.Body.Tags,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
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
		return nil, httpapi.Unauthorized()
	}

	if err := h.feed.deletePost(ctx, userID, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}

type FeaturedPost struct {
	ID         uuid.UUID  `json:"id" format:"uuid"`
	Title      string     `json:"title"`
	ImageURL   string     `json:"imageUrl" doc:"First available post image"`
	University string     `json:"university"`
	EventDate  *time.Time `json:"eventDate" nullable:"true"`
}

type featuredPostsOutput struct {
	Body struct {
		Items []FeaturedPost `json:"items"`
	}
}

var featuredPostsOperation = auth.Protected(huma.Operation{
	OperationID: "listFeaturedFeedPosts",
	Method:      http.MethodGet,
	Path:        "/feed/posts/featured",
	Summary:     "Get featured feed posts",
	Description: "Up to five posts visible in the regular feed, ranked by likes received during the current calendar week (Monday 00:00 to next Monday 00:00, Europe/Warsaw). Posts of any age and type qualify with at least one remaining like from that week and at least one ready image. Ties use createdAt and ID descending. Rankings update when likes are added or removed; posts leave on deletion, when outranked, when the last image is gone, or when the week changes. Every item has an image. An empty section returns items: [].",
	Tags:        []string{"feed"},
})

func (h *postHandler) featuredPosts(ctx context.Context, _ *struct{}) (*featuredPostsOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	cards, err := h.feed.featuredPosts(ctx, viewerID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &featuredPostsOutput{}
	out.Body.Items = cards
	return out, nil
}

type PostNavigation struct {
	PreviousID *uuid.UUID `json:"previousId" format:"uuid" nullable:"true" doc:"Nearest newer post, or null at the start"`
	NextID     *uuid.UUID `json:"nextId" format:"uuid" nullable:"true" doc:"Nearest older post, or null at the end"`
}

type adjacentPostsInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
	PostFilters
}

type adjacentPostsOutput struct {
	Body PostNavigation
}

var adjacentPostsOperation = auth.Protected(huma.Operation{
	OperationID: "getFeedPostNavigation",
	Method:      http.MethodGet,
	Path:        "/feed/posts/{id}/navigation",
	Summary:     "Get previous and next feed post IDs",
	Description: "Uses the regular feed visibility and createdAt/ID descending order. Pass the active feed q, authorId, university, type, from and to filters; omit them to navigate the whole feed. Returns 404 if the current post is deleted, inaccessible, or outside these filters. Both IDs are null for a single matching post.",
	Tags:        []string{"feed"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *postHandler) adjacentPosts(ctx context.Context, in *adjacentPostsInput) (*adjacentPostsOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	navigation, err := h.feed.adjacentPosts(ctx, in.ID, in.query(userID))
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &adjacentPostsOutput{Body: *navigation}, nil
}
