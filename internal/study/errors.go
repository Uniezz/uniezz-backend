package study

import (
	"errors"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrInvalidInput      = errors.New("invalid input")
	ErrContentNotAllowed = errors.New("content not allowed")
	ErrInvalidMedia      = errors.New("media is missing, not owned by the user or not uploaded")
	ErrRateLimited       = errors.New("too many new courses, files or comments")
	ErrCourseExists      = errors.New("the faculty already has a course with this name")
	ErrSimilarCourses    = errors.New("the faculty has courses with similar names")
)

var errorResponses = httpapi.ErrorResponses{
	{Err: ErrNotFound, Status: http.StatusNotFound, Code: "not_found"},
	{Err: ErrInvalidInput, Status: http.StatusBadRequest, Code: "invalid_input"},
	{Err: ErrContentNotAllowed, Status: http.StatusBadRequest, Code: "content_not_allowed"},
	{Err: ErrInvalidMedia, Status: http.StatusBadRequest, Code: "invalid_media"},
	{Err: ErrRateLimited, Status: http.StatusTooManyRequests, Code: "rate_limited"},
	{Err: ErrCourseExists, Status: http.StatusConflict, Code: "course_exists"},
	{Err: ErrSimilarCourses, Status: http.StatusConflict, Code: "similar_courses"},
	{Err: httpapi.ErrInvalidCursor, Status: http.StatusBadRequest, Code: "invalid_cursor"},
}
