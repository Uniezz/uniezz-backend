package media

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
)

type uploadHandler struct {
	media Module
}

type createUploadInput struct {
	Body struct {
		Kind        Kind   `json:"kind" enum:"avatar,feed,chat" doc:"What the file will be used for"`
		ContentType string `json:"contentType" enum:"image/jpeg,image/png,image/webp" doc:"MIME type the file will be uploaded with"`
		Size        int64  `json:"size" minimum:"1" doc:"Exact file size in bytes; 5 MiB max for avatars, 10 MiB for feed and chat"`
	}
}

type createUploadOutput struct {
	Body Upload
}

var createUploadOperation = auth.Protected(huma.Operation{
	OperationID:   "createMediaUpload",
	Method:        http.MethodPost,
	Path:          "/media/uploads",
	Summary:       "Start a media upload",
	Description:   "Creates a pending media item owned by the caller and returns a presigned URL to upload the file to.",
	Tags:          []string{"media"},
	DefaultStatus: http.StatusCreated,
	Errors:        []int{http.StatusBadRequest},
})

func (h *uploadHandler) createUpload(ctx context.Context, in *createUploadInput) (*createUploadOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Error(http.StatusUnauthorized, "unauthorized", nil)
	}

	upload, err := h.media.CreateUpload(ctx, userID, in.Body.Kind, in.Body.ContentType, in.Body.Size)
	if err != nil {
		return nil, apiError(err)
	}
	return &createUploadOutput{Body: upload}, nil
}
