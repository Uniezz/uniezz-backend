package study

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
	study Module
}

type CourseComment struct {
	ID        uuid.UUID `json:"id" format:"uuid"`
	CourseID  uuid.UUID `json:"courseId" format:"uuid"`
	Content   string    `json:"content" example:"Hard, but the lectures help a lot."`
	IsMine    bool      `json:"isMine" doc:"True when the signed-in student wrote the comment and may delete it"`
	CreatedAt time.Time `json:"createdAt"`
}

type addCourseCommentInput struct {
	ID   uuid.UUID `path:"id" format:"uuid"`
	Body struct {
		Content string `json:"content" minLength:"1" maxLength:"4000" doc:"Up to 1000 characters after HTML is stripped" example:"Hard, but the lectures help a lot."`
	}
}

type addCourseCommentOutput struct {
	Body CourseComment
}

var addCourseCommentOperation = auth.Protected(huma.Operation{
	OperationID:   "addCourseComment",
	Method:        http.MethodPost,
	Path:          "/study/courses/{id}/comments",
	Summary:       "Comment on a course",
	Description:   "Comments are shown without their author. Counts toward the limit of 30 creates an hour.",
	Tags:          []string{"study"},
	DefaultStatus: http.StatusCreated,
	Errors:        []int{http.StatusBadRequest, http.StatusNotFound, http.StatusTooManyRequests},
})

func (h *commentHandler) addCourseComment(ctx context.Context, in *addCourseCommentInput) (*addCourseCommentOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	comment, err := h.study.addComment(ctx, userID, commentChange{CourseID: in.ID, Content: in.Body.Content})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &addCourseCommentOutput{Body: *comment}, nil
}

type listCourseCommentsInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
	httpapi.PageQuery
}

type listCourseCommentsOutput struct {
	Body struct {
		Items      []CourseComment `json:"items"`
		NextCursor *string         `json:"nextCursor" nullable:"true" doc:"Null on the last page"`
	}
}

var listCourseCommentsOperation = auth.Protected(huma.Operation{
	OperationID: "listCourseComments",
	Method:      http.MethodGet,
	Path:        "/study/courses/{id}/comments",
	Summary:     "List the comments on a course",
	Description: "Oldest first. Comments of blocked students are left out.",
	Tags:        []string{"study"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *commentHandler) listCourseComments(ctx context.Context, in *listCourseCommentsInput) (*listCourseCommentsOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	items, next, err := h.study.listComments(ctx, commentQuery{
		ViewerID: viewerID,
		CourseID: in.ID,
		Cursor:   in.Cursor,
		Limit:    in.Limit,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &listCourseCommentsOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

type deleteCommentInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

var deleteCommentOperation = auth.Protected(huma.Operation{
	OperationID:   "deleteComment",
	Method:        http.MethodDelete,
	Path:          "/study/comments/{id}",
	Summary:       "Delete the student's own comment on a course",
	Description:   "A comment that is already gone, or that another student wrote, is left as it is and the call still succeeds.",
	Tags:          []string{"study"},
	DefaultStatus: http.StatusNoContent,
})

func (h *commentHandler) deleteComment(ctx context.Context, in *deleteCommentInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	if err := h.study.deleteComment(ctx, userID, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}
