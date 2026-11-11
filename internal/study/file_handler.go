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

type fileHandler struct {
	study Module
}

type CourseFile struct {
	ID        uuid.UUID `json:"id" format:"uuid"`
	CourseID  uuid.UUID `json:"courseId" format:"uuid"`
	AuthorID  uuid.UUID `json:"authorId" format:"uuid"`
	Title     string    `json:"title" example:"Midterm 2024, group A"`
	Type      FileType  `json:"type" enum:"exam,colloquium,lab,notes"`
	URL       string    `json:"url" doc:"Short-lived link to the PDF or image"`
	CreatedAt time.Time `json:"createdAt"`
}

type addCourseFileInput struct {
	ID   uuid.UUID `path:"id" format:"uuid"`
	Body struct {
		MediaID uuid.UUID `json:"mediaId" format:"uuid" doc:"Upload created with POST /media/uploads and kind exam"`
		Title   string    `json:"title" minLength:"1" maxLength:"600" doc:"3 to 150 characters after HTML is stripped" example:"Midterm 2024, group A"`
		Type    FileType  `json:"type" enum:"exam,colloquium,lab,notes"`
	}
}

type addCourseFileOutput struct {
	Body CourseFile
}

var addCourseFileOperation = auth.Protected(huma.Operation{
	OperationID:   "addCourseFile",
	Method:        http.MethodPost,
	Path:          "/study/courses/{id}/files",
	Summary:       "Attach a file to a course",
	Description:   "The file is a PDF up to 20 MB or a JPEG, PNG or WebP up to 10 MB, uploaded with kind exam. Counts toward the limit of 30 creates an hour.",
	Tags:          []string{"study"},
	DefaultStatus: http.StatusCreated,
	Errors:        []int{http.StatusBadRequest, http.StatusNotFound, http.StatusTooManyRequests},
})

func (h *fileHandler) addCourseFile(ctx context.Context, in *addCourseFileInput) (*addCourseFileOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	file, err := h.study.addFile(ctx, userID, fileChange{
		CourseID: in.ID,
		MediaID:  in.Body.MediaID,
		Title:    in.Body.Title,
		Type:     in.Body.Type,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &addCourseFileOutput{Body: *file}, nil
}

type listCourseFilesInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
	httpapi.PageQuery
}

type listCourseFilesOutput struct {
	Body struct {
		Items      []CourseFile `json:"items"`
		NextCursor *string      `json:"nextCursor" nullable:"true" doc:"Null on the last page"`
	}
}

var listCourseFilesOperation = auth.Protected(huma.Operation{
	OperationID: "listCourseFiles",
	Method:      http.MethodGet,
	Path:        "/study/courses/{id}/files",
	Summary:     "List the files of a course",
	Description: "Newest first. Files of blocked students are left out.",
	Tags:        []string{"study"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *fileHandler) listCourseFiles(ctx context.Context, in *listCourseFilesInput) (*listCourseFilesOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	items, next, err := h.study.listCourseFiles(ctx, fileQuery{
		ViewerID: viewerID,
		CourseID: in.ID,
		Cursor:   in.Cursor,
		Limit:    in.Limit,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &listCourseFilesOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

type searchFilesInput struct {
	httpapi.PageQuery
	Search       string `query:"q" maxLength:"200" doc:"Up to 5 words of at least 3 characters, matched against the file title and the course subject" example:"midterm algebra"`
	UniversityID string `query:"universityId" maxLength:"32" example:"umcs"`
	Faculty      string `query:"faculty" maxLength:"600" doc:"Exact faculty name of the course"`
	Subject      string `query:"subject" maxLength:"600" doc:"Exact subject name of the course"`
	Semester     int    `query:"semester" minimum:"0" maximum:"10" doc:"0 or omitted for every semester"`
	Mine         bool   `query:"mine" doc:"Only files uploaded by the signed-in student"`
}

type searchFilesOutput struct {
	Body struct {
		Items      []CourseFile `json:"items"`
		NextCursor *string      `json:"nextCursor" nullable:"true" doc:"Null on the last page"`
	}
}

var searchFilesOperation = auth.Protected(huma.Operation{
	OperationID: "searchFiles",
	Method:      http.MethodGet,
	Path:        "/study/files",
	Summary:     "Search files of all courses",
	Description: "Newest first. Filters combine and apply to the course of the file. Files of blocked students are left out.",
	Tags:        []string{"study"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *fileHandler) searchFiles(ctx context.Context, in *searchFilesInput) (*searchFilesOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	items, next, err := h.study.searchFiles(ctx, fileQuery{
		ViewerID:     viewerID,
		Search:       in.Search,
		UniversityID: in.UniversityID,
		Faculty:      in.Faculty,
		Subject:      in.Subject,
		Semester:     in.Semester,
		Mine:         in.Mine,
		Cursor:       in.Cursor,
		Limit:        in.Limit,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &searchFilesOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

type getFileInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

type getFileOutput struct {
	Body CourseFile
}

var getFileOperation = auth.Protected(huma.Operation{
	OperationID: "getFile",
	Method:      http.MethodGet,
	Path:        "/study/files/{id}",
	Summary:     "Get a file",
	Description: "A file of a blocked student is not found.",
	Tags:        []string{"study"},
	Errors:      []int{http.StatusNotFound},
})

func (h *fileHandler) getFile(ctx context.Context, in *getFileInput) (*getFileOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	file, err := h.study.getFile(ctx, viewerID, in.ID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &getFileOutput{Body: *file}, nil
}

type deleteFileInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

var deleteFileOperation = auth.Protected(huma.Operation{
	OperationID:   "deleteFile",
	Method:        http.MethodDelete,
	Path:          "/study/files/{id}",
	Summary:       "Delete the student's own file",
	Description:   "Deletes the file and its upload. A file that is already gone, or that another student uploaded, is left as it is and the call still succeeds.",
	Tags:          []string{"study"},
	DefaultStatus: http.StatusNoContent,
})

func (h *fileHandler) deleteFile(ctx context.Context, in *deleteFileInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	if err := h.study.deleteFile(ctx, userID, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}
