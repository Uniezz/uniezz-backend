package study

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
)

type FileType string

const (
	FileTypeExam       FileType = "exam"
	FileTypeColloquium FileType = "colloquium"
	FileTypeLab        FileType = "lab"
	FileTypeNotes      FileType = "notes"
)

func (t FileType) valid() bool {
	switch t {
	case FileTypeExam, FileTypeColloquium, FileTypeLab, FileTypeNotes:
		return true
	default:
		return false
	}
}

type fileChange struct {
	CourseID uuid.UUID
	MediaID  uuid.UUID
	Title    string
	Type     FileType
}

func (c fileChange) fields() (fileFields, error) {
	title, err := cleanName(c.Title)
	if err != nil {
		return fileFields{}, err
	}
	if !c.Type.valid() {
		return fileFields{}, ErrInvalidInput
	}
	return fileFields{CourseID: c.CourseID, MediaID: c.MediaID, Title: title, Type: c.Type}, nil
}

type fileQuery struct {
	ViewerID     uuid.UUID
	CourseID     uuid.UUID
	Search       string
	UniversityID string
	Faculty      string
	Subject      string
	Semester     int
	Mine         bool
	Cursor       string
	Limit        int
}

func (q fileQuery) filter() (fileFilter, error) {
	if q.UniversityID != "" {
		if _, err := auth.GetUniversityByID(auth.UniversityID(q.UniversityID)); err != nil {
			return fileFilter{}, ErrInvalidInput
		}
	}
	if q.Semester != 0 && (q.Semester < minSemester || q.Semester > maxSemester) {
		return fileFilter{}, ErrInvalidInput
	}
	faculty, err := cleanFilterValue(q.Faculty)
	if err != nil {
		return fileFilter{}, err
	}
	subject, err := cleanFilterValue(q.Subject)
	if err != nil {
		return fileFilter{}, err
	}
	words, err := text.SearchWords(q.Search)
	if err != nil {
		return fileFilter{}, ErrInvalidInput
	}
	after, err := decodePageCursor(q.Cursor)
	if err != nil {
		return fileFilter{}, err
	}
	f := fileFilter{
		ViewerID:     q.ViewerID,
		CourseID:     q.CourseID,
		Words:        words,
		UniversityID: q.UniversityID,
		Faculty:      faculty,
		Subject:      subject,
		Semester:     q.Semester,
		After:        after,
		Limit:        q.Limit + 1,
	}
	if q.Mine {
		f.AuthorID = q.ViewerID
	}
	return f, nil
}

func (m Module) addFile(ctx context.Context, userID uuid.UUID, change fileChange) (*CourseFile, error) {
	fields, err := change.fields()
	if err != nil {
		return nil, err
	}
	if err := m.media.CheckUpload(ctx, fields.MediaID, userID, media.KindExam); err != nil {
		return nil, mediaError(err)
	}

	var id uuid.UUID
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.allowCreate(ctx, userID); err != nil {
			return err
		}
		if err := m.media.MarkReady(ctx, fields.MediaID, userID, media.KindExam); err != nil {
			return mediaError(err)
		}
		id, err = m.files.create(ctx, userID, fields)
		return err
	})
	if err != nil {
		return nil, err
	}
	return m.getFile(ctx, userID, id)
}

func (m Module) getFile(ctx context.Context, viewerID, id uuid.UUID) (*CourseFile, error) {
	row, err := m.files.get(ctx, viewerID, id)
	if err != nil {
		return nil, err
	}
	file, err := m.toFile(ctx, *row)
	if err != nil {
		return nil, err
	}
	return &file, nil
}

func (m Module) listCourseFiles(ctx context.Context, q fileQuery) ([]CourseFile, *string, error) {
	if _, err := m.courses.get(ctx, q.CourseID); err != nil {
		return nil, nil, err
	}
	return m.searchFiles(ctx, q)
}

func (m Module) searchFiles(ctx context.Context, q fileQuery) ([]CourseFile, *string, error) {
	filter, err := q.filter()
	if err != nil {
		return nil, nil, err
	}
	rows, err := m.files.list(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	rows, next, err := httpapi.Page(rows, q.Limit, func(last fileRow) any {
		return pageCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	})
	if err != nil {
		return nil, nil, err
	}
	files := make([]CourseFile, 0, len(rows))
	for _, row := range rows {
		file, err := m.toFile(ctx, row)
		if err != nil {
			return nil, nil, err
		}
		files = append(files, file)
	}
	return files, next, nil
}

func (m Module) deleteFile(ctx context.Context, userID, id uuid.UUID) error {
	var mediaID uuid.UUID
	var found bool
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		mediaID, found, err = m.files.delete(ctx, id, userID)
		if err != nil || !found {
			return err
		}
		err = m.media.MarkDeleting(ctx, mediaID)
		if errors.Is(err, media.ErrNotFound) {
			found = false
			return nil
		}
		return err
	})
	if err != nil || !found {
		return err
	}

	if err := m.media.Delete(ctx, mediaID); err != nil {
		slog.WarnContext(ctx, "delete course file media, left for cleanup", slog.String("media_id", mediaID.String()), slog.Any("error", err))
	}
	return nil
}

func (m Module) toFile(ctx context.Context, row fileRow) (CourseFile, error) {
	url, err := m.media.URLOf(ctx, media.Ref{Kind: row.MediaKind, ObjectKey: row.ObjectKey})
	if err != nil {
		return CourseFile{}, err
	}
	return CourseFile{
		ID:        row.ID,
		CourseID:  row.CourseID,
		AuthorID:  row.AuthorID,
		Title:     row.Title,
		Type:      row.Type,
		URL:       url,
		CreatedAt: row.CreatedAt,
	}, nil
}

func mediaError(err error) error {
	if errors.Is(err, media.ErrNotFound) || errors.Is(err, media.ErrUploadMismatch) {
		return errors.Join(ErrInvalidMedia, err)
	}
	return err
}
