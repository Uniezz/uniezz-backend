package media

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
)

type uploadHandler struct {
	media Module
}

type uploadInput struct {
	Body struct {
		Kind        Kind   `json:"kind" enum:"avatar,feed,chat" doc:"What the file will be used for"`
		ContentType string `json:"contentType" enum:"image/jpeg,image/png,image/webp" doc:"MIME type the file will be uploaded with"`
		Size        int64  `json:"size" minimum:"1" doc:"Exact file size in bytes; 5 MiB max for avatars, 10 MiB for feed and chat"`
	}
}

type uploadOutput struct {
	Body struct {
		MediaID   string    `json:"mediaId" format:"uuid" doc:"ID to pass to the endpoint that uses the file, e.g. avatarMediaId"`
		UploadURL string    `json:"uploadUrl" doc:"Presigned URL to PUT the file to, with the same Content-Type and Content-Length"`
		ExpiresAt time.Time `json:"expiresAt" doc:"When the upload URL stops working"`
	}
}

var uploadOperation = auth.Protected(huma.Operation{
	OperationID:   "createMediaUpload",
	Method:        http.MethodPost,
	Path:          "/media/uploads",
	Summary:       "Start a media upload",
	Description:   "Creates a pending media item owned by the caller and returns a presigned URL to upload the file to.",
	Tags:          []string{"media"},
	DefaultStatus: http.StatusCreated,
	Errors:        []int{http.StatusBadRequest},
})

func (h *uploadHandler) create(ctx context.Context, in *uploadInput) (*uploadOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Error(http.StatusUnauthorized, "unauthorized", nil)
	}

	upload, err := h.media.CreateUpload(ctx, userID, in.Body.Kind, in.Body.ContentType, in.Body.Size)
	if err != nil {
		if errors.Is(err, ErrInvalidUpload) {
			return nil, httpapi.Error(http.StatusBadRequest, "invalid_upload", err)
		}
		return nil, httpapi.Error(http.StatusInternalServerError, "internal_error", err)
	}

	out := &uploadOutput{}
	out.Body.MediaID = upload.MediaID.String()
	out.Body.UploadURL = upload.UploadURL
	out.Body.ExpiresAt = upload.ExpiresAt
	return out, nil
}
