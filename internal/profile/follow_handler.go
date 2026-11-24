package profile

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type followHandler struct {
	profile Module
}

type followProfileInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

var followProfileOperation = auth.Authenticated(huma.Operation{
	OperationID:   "followProfile",
	Method:        http.MethodPut,
	Path:          "/profiles/{id}/follow",
	Summary:       "Follow a student",
	Description:   "Creates a public, one-way follow. A follow is not a Connection and does not create a chat or notification. Following the same student again succeeds without creating another follow. A blocked profile is not found.",
	Tags:          []string{"profile"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *followHandler) follow(ctx context.Context, in *followProfileInput) (*struct{}, error) {
	followerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	if err := h.profile.follow(ctx, followerID, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}

var unfollowProfileOperation = auth.Authenticated(huma.Operation{
	OperationID:   "unfollowProfile",
	Method:        http.MethodDelete,
	Path:          "/profiles/{id}/follow",
	Summary:       "Unfollow a student",
	Description:   "Removes the caller's follow. Unfollowing a student who is not followed succeeds.",
	Tags:          []string{"profile"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *followHandler) unfollow(ctx context.Context, in *followProfileInput) (*struct{}, error) {
	followerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	if err := h.profile.unfollow(ctx, followerID, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}

type listMyFollowsInput struct {
	httpapi.PageQuery
}

type listProfileFollowsInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
	httpapi.PageQuery
}

type listFollowsOutput struct {
	Body struct {
		Items      []ProfileSummary `json:"items"`
		NextCursor *string          `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get the next page; null on the last page"`
	}
}

var listMyFollowersOperation = auth.Authenticated(huma.Operation{
	OperationID: "listMyFollowers",
	Method:      http.MethodGet,
	Path:        "/profile/me/followers",
	Summary:     "List students who follow the logged-in student",
	Description: "Newest follow first, paginated by cursor.",
	Tags:        []string{"profile"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *followHandler) listMyFollowers(ctx context.Context, in *listMyFollowsInput) (*listFollowsOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	return h.list(ctx, viewerID, viewerID, false, in.Cursor, in.Limit)
}

var listMyFollowingOperation = auth.Authenticated(huma.Operation{
	OperationID: "listMyFollowing",
	Method:      http.MethodGet,
	Path:        "/profile/me/following",
	Summary:     "List students followed by the logged-in student",
	Description: "Newest follow first, paginated by cursor.",
	Tags:        []string{"profile"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *followHandler) listMyFollowing(ctx context.Context, in *listMyFollowsInput) (*listFollowsOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	return h.list(ctx, viewerID, viewerID, true, in.Cursor, in.Limit)
}

var listProfileFollowersOperation = auth.Authenticated(huma.Operation{
	OperationID: "listProfileFollowers",
	Method:      http.MethodGet,
	Path:        "/profiles/{id}/followers",
	Summary:     "List students who follow a profile",
	Description: "Newest follow first, paginated by cursor.",
	Tags:        []string{"profile"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *followHandler) listProfileFollowers(ctx context.Context, in *listProfileFollowsInput) (*listFollowsOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	return h.list(ctx, viewerID, in.ID, false, in.Cursor, in.Limit)
}

var listProfileFollowingOperation = auth.Authenticated(huma.Operation{
	OperationID: "listProfileFollowing",
	Method:      http.MethodGet,
	Path:        "/profiles/{id}/following",
	Summary:     "List profiles followed by a student",
	Description: "Newest follow first, paginated by cursor.",
	Tags:        []string{"profile"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *followHandler) listProfileFollowing(ctx context.Context, in *listProfileFollowsInput) (*listFollowsOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	return h.list(ctx, viewerID, in.ID, true, in.Cursor, in.Limit)
}

func (h *followHandler) list(
	ctx context.Context,
	viewerID, profileID uuid.UUID,
	following bool,
	cursor string,
	limit int,
) (*listFollowsOutput, error) {
	items, next, err := h.profile.listFollows(ctx, followListQuery{
		ViewerID:  viewerID,
		ProfileID: profileID,
		Following: following,
		Cursor:    cursor,
		Limit:     limit,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}

	out := &listFollowsOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}
