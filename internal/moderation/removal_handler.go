package moderation

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type removalHandler struct {
	moderation Module
}

type removeContentInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

var removePostOperation = auth.Authorized(huma.Operation{
	OperationID:   "removePost",
	Method:        http.MethodDelete,
	Path:          "/moderation/posts/{id}",
	Summary:       "Remove any student's post",
	Description:   "Deletes the post with its comments, likes and media, and marks its pending reports reviewed. Removing a post that is already gone succeeds. Moderators and admins only.",
	Tags:          []string{"moderation"},
	DefaultStatus: http.StatusNoContent,
}, auth.RoleModerator, auth.RoleAdmin)

func (h *removalHandler) removePost(ctx context.Context, in *removeContentInput) (*struct{}, error) {
	if err := h.moderation.removeContent(ctx, targetPost, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}

var removeCommentOperation = auth.Authorized(huma.Operation{
	OperationID:   "removeComment",
	Method:        http.MethodDelete,
	Path:          "/moderation/comments/{id}",
	Summary:       "Remove any student's comment",
	Description:   "Deletes the comment, or leaves an anonymous placeholder when it still has replies, and marks its pending reports reviewed. Removing a comment that is already gone succeeds. Moderators and admins only.",
	Tags:          []string{"moderation"},
	DefaultStatus: http.StatusNoContent,
}, auth.RoleModerator, auth.RoleAdmin)

func (h *removalHandler) removeComment(ctx context.Context, in *removeContentInput) (*struct{}, error) {
	if err := h.moderation.removeContent(ctx, targetComment, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}

var removeFileOperation = auth.Authorized(huma.Operation{
	OperationID:   "removeCourseFile",
	Method:        http.MethodDelete,
	Path:          "/moderation/files/{id}",
	Summary:       "Remove any student's exam file",
	Description:   "Deletes the file and marks its pending reports reviewed. Removing a file that is already gone succeeds. Moderators and admins only.",
	Tags:          []string{"moderation"},
	DefaultStatus: http.StatusNoContent,
}, auth.RoleModerator, auth.RoleAdmin)

func (h *removalHandler) removeFile(ctx context.Context, in *removeContentInput) (*struct{}, error) {
	if err := h.moderation.removeContent(ctx, targetFile, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}

var removeCourseOperation = auth.Authorized(huma.Operation{
	OperationID:   "removeCourse",
	Method:        http.MethodDelete,
	Path:          "/moderation/courses/{id}",
	Summary:       "Remove any course",
	Description:   "Deletes the course with every student's files and comments on it, and marks its pending reports reviewed. Removing a course that is already gone succeeds. Admins only.",
	Tags:          []string{"moderation"},
	DefaultStatus: http.StatusNoContent,
}, auth.RoleAdmin)

func (h *removalHandler) removeCourse(ctx context.Context, in *removeContentInput) (*struct{}, error) {
	if err := h.moderation.removeContent(ctx, targetCourse, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}
