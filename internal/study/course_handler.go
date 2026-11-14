package study

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type courseHandler struct {
	study Module
}

type Course struct {
	ID            uuid.UUID `json:"id" format:"uuid"`
	Name          string    `json:"name" example:"Calculus I"`
	Description   string    `json:"description" example:"Limits, derivatives and integrals of one variable."`
	Difficulty    int       `json:"difficulty" minimum:"1" maximum:"5" example:"4"`
	UniversityID  string    `json:"universityId" example:"umcs"`
	Faculty       string    `json:"faculty" example:"Faculty of Mathematics, Physics and Computer Science"`
	Subject       string    `json:"subject" example:"Computer Science"`
	Semester      int       `json:"semester" minimum:"1" maximum:"10" example:"1"`
	RatingAverage float64   `json:"ratingAverage" doc:"Average student rating from 1 to 5, 0 when nobody rated yet" example:"4.2"`
	RatingCount   int       `json:"ratingCount" example:"17"`
	IsMine        bool      `json:"isMine" doc:"True when the signed-in student created the course and may edit it"`
}

type createCourseInput struct {
	Body struct {
		Name         string `json:"name" minLength:"1" maxLength:"600" doc:"3 to 150 characters after HTML is stripped" example:"Calculus I"`
		Description  string `json:"description" minLength:"1" maxLength:"20000" doc:"Up to 5000 characters after HTML is stripped"`
		Difficulty   int    `json:"difficulty" minimum:"1" maximum:"5" example:"4"`
		UniversityID string `json:"universityId" minLength:"1" maxLength:"32" example:"umcs"`
		Faculty      string `json:"faculty" minLength:"1" maxLength:"600" doc:"3 to 150 characters after HTML is stripped"`
		Subject      string `json:"subject" minLength:"1" maxLength:"600" doc:"3 to 150 characters after HTML is stripped" example:"Computer Science"`
		Semester     int    `json:"semester" minimum:"1" maximum:"10" example:"1"`
	}
}

type createCourseOutput struct {
	Body Course
}

var createCourseOperation = auth.Protected(huma.Operation{
	OperationID:   "createCourse",
	Method:        http.MethodPost,
	Path:          "/study/courses",
	Summary:       "Create a course",
	Description:   "Adds a course to the catalogue of a university. A student may create, upload and comment up to 30 times an hour in total.",
	Tags:          []string{"study"},
	DefaultStatus: http.StatusCreated,
	Errors:        []int{http.StatusBadRequest, http.StatusTooManyRequests},
})

func (h *courseHandler) createCourse(ctx context.Context, in *createCourseInput) (*createCourseOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	course, err := h.study.createCourse(ctx, userID, courseChange{
		Name:         in.Body.Name,
		Description:  in.Body.Description,
		Difficulty:   in.Body.Difficulty,
		UniversityID: in.Body.UniversityID,
		Faculty:      in.Body.Faculty,
		Subject:      in.Body.Subject,
		Semester:     in.Body.Semester,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &createCourseOutput{Body: *course}, nil
}

type listCoursesInput struct {
	httpapi.PageQuery
	UniversityID string `query:"universityId" maxLength:"32" example:"umcs"`
	Faculty      string `query:"faculty" maxLength:"600" doc:"Exact faculty name"`
	Subject      string `query:"subject" maxLength:"600" doc:"Exact subject name"`
	Semester     int    `query:"semester" minimum:"0" maximum:"10" doc:"0 or omitted for every semester"`
}

type listCoursesOutput struct {
	Body struct {
		Items      []Course `json:"items"`
		NextCursor *string  `json:"nextCursor" nullable:"true" doc:"Null on the last page"`
	}
}

var listCoursesOperation = auth.Protected(huma.Operation{
	OperationID: "listCourses",
	Method:      http.MethodGet,
	Path:        "/study/courses",
	Summary:     "List courses",
	Description: "Newest first. Filters combine: a course must match every filter that is set.",
	Tags:        []string{"study"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *courseHandler) listCourses(ctx context.Context, in *listCoursesInput) (*listCoursesOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	items, next, err := h.study.listCourses(ctx, courseQuery{
		ViewerID:     viewerID,
		UniversityID: in.UniversityID,
		Faculty:      in.Faculty,
		Subject:      in.Subject,
		Semester:     in.Semester,
		Cursor:       in.Cursor,
		Limit:        in.Limit,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &listCoursesOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

type getCourseInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

type getCourseOutput struct {
	Body Course
}

var getCourseOperation = auth.Protected(huma.Operation{
	OperationID: "getCourse",
	Method:      http.MethodGet,
	Path:        "/study/courses/{id}",
	Summary:     "Get a course",
	Description: "Returns the course with its average rating.",
	Tags:        []string{"study"},
	Errors:      []int{http.StatusNotFound},
})

func (h *courseHandler) getCourse(ctx context.Context, in *getCourseInput) (*getCourseOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	course, err := h.study.getCourse(ctx, viewerID, in.ID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &getCourseOutput{Body: *course}, nil
}

type updateCourseInput struct {
	ID   uuid.UUID `path:"id" format:"uuid"`
	Body struct {
		Name        *string `json:"name,omitempty" minLength:"1" maxLength:"600" doc:"3 to 150 characters after HTML is stripped" example:"Calculus I"`
		Description *string `json:"description,omitempty" minLength:"1" maxLength:"20000" doc:"Up to 5000 characters after HTML is stripped"`
		Difficulty  *int    `json:"difficulty,omitempty" minimum:"1" maximum:"5" example:"4"`
	}
}

type updateCourseOutput struct {
	Body Course
}

var updateCourseOperation = auth.Protected(huma.Operation{
	OperationID: "updateCourse",
	Method:      http.MethodPatch,
	Path:        "/study/courses/{id}",
	Summary:     "Update the name, description or difficulty of the student's own course",
	Description: "Only the student who created the course may change it; for anyone else the course is not found. Omitted fields stay unchanged.",
	Tags:        []string{"study"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *courseHandler) updateCourse(ctx context.Context, in *updateCourseInput) (*updateCourseOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	course, err := h.study.updateCourse(ctx, userID, in.ID, courseEditChange{
		Name:        in.Body.Name,
		Description: in.Body.Description,
		Difficulty:  in.Body.Difficulty,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &updateCourseOutput{Body: *course}, nil
}

type deleteCourseInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

var deleteCourseOperation = auth.Protected(huma.Operation{
	OperationID:   "deleteCourse",
	Method:        http.MethodDelete,
	Path:          "/study/courses/{id}",
	Summary:       "Delete the student's own course with its files, comments and ratings",
	Description:   "Deletes the course with the files, comments and ratings every student left on it. A course that is already gone, or that another student created, is left as it is and the call still succeeds.",
	Tags:          []string{"study"},
	DefaultStatus: http.StatusNoContent,
})

func (h *courseHandler) deleteCourse(ctx context.Context, in *deleteCourseInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	if err := h.study.deleteCourse(ctx, userID, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}

type rateCourseInput struct {
	ID   uuid.UUID `path:"id" format:"uuid"`
	Body struct {
		Rating int `json:"rating" minimum:"1" maximum:"5" example:"4"`
	}
}

type rateCourseOutput struct {
	Body Course
}

var rateCourseOperation = auth.Protected(huma.Operation{
	OperationID: "rateCourse",
	Method:      http.MethodPut,
	Path:        "/study/courses/{id}/rating",
	Summary:     "Rate a course",
	Description: "One rating per student; rating again replaces the previous one. Returns the course with its new average.",
	Tags:        []string{"study"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *courseHandler) rateCourse(ctx context.Context, in *rateCourseInput) (*rateCourseOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	course, err := h.study.rateCourse(ctx, userID, in.ID, in.Body.Rating)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &rateCourseOutput{Body: *course}, nil
}
