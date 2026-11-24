package media

import (
	"context"
	"net/http"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type imageHandler struct {
	media Module
}

type Image struct {
	ID        uuid.UUID `json:"id" format:"uuid"`
	Kind      Kind      `json:"kind" enum:"avatar,banner,feed"`
	URL       string    `json:"url"`
	OwnerID   uuid.UUID `json:"ownerId" format:"uuid"`
	CreatedAt time.Time `json:"createdAt"`
}

type listUnreviewedImagesInput struct {
	httpapi.PageQuery
}

type listUnreviewedImagesOutput struct {
	Body struct {
		Items      []Image `json:"items"`
		NextCursor *string `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get the next page; null on the last page"`
	}
}

var listUnreviewedImagesOperation = auth.Authorized(huma.Operation{
	OperationID: "listUnreviewedImages",
	Method:      http.MethodGet,
	Path:        "/media/images",
	Summary:     "List the avatars, banners and feed images no moderator has checked yet",
	Description: "Oldest first, paginated by cursor. An image leaves the list once it is marked checked or removed. Private chat images are left out. Moderators and admins only.",
	Tags:        []string{"media"},
	Errors:      []int{http.StatusBadRequest},
}, auth.RoleModerator, auth.RoleAdmin)

func (h *imageHandler) listUnreviewed(ctx context.Context, in *listUnreviewedImagesInput) (*listUnreviewedImagesOutput, error) {
	items, next, err := h.media.listUnreviewed(ctx, in.Cursor, in.Limit)
	if err != nil {
		return nil, errorResponses.Error(err)
	}

	out := &listUnreviewedImagesOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

type imageInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

var markImageReviewedOperation = auth.Authorized(huma.Operation{
	OperationID:   "markImageReviewed",
	Method:        http.MethodPut,
	Path:          "/media/images/{id}/review",
	Summary:       "Mark an avatar, banner or feed image as checked",
	Description:   "Marking it again changes nothing. Moderators and admins only.",
	Tags:          []string{"media"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusNotFound},
}, auth.RoleModerator, auth.RoleAdmin)

func (h *imageHandler) markReviewed(ctx context.Context, in *imageInput) (*struct{}, error) {
	if err := h.media.markReviewed(ctx, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}

var removeImageOperation = auth.Authorized(huma.Operation{
	OperationID:   "removeImage",
	Method:        http.MethodDelete,
	Path:          "/media/images/{id}",
	Summary:       "Remove any student's avatar, banner or feed image",
	Description:   "The image disappears everywhere it was shown and its file is deleted. Removing it again succeeds. Moderators and admins only.",
	Tags:          []string{"media"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusNotFound},
}, auth.RoleModerator, auth.RoleAdmin)

func (h *imageHandler) removeImage(ctx context.Context, in *imageInput) (*struct{}, error) {
	if err := h.media.removeImage(ctx, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}
