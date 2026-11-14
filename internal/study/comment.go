package study

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/google/uuid"
)

type commentChange struct {
	CourseID uuid.UUID
	Content  string
}

type commentQuery struct {
	ViewerID uuid.UUID
	CourseID uuid.UUID
	Cursor   string
	Limit    int
}

func (q commentQuery) filter() (commentFilter, error) {
	after, err := decodePageCursor(q.Cursor)
	if err != nil {
		return commentFilter{}, err
	}
	return commentFilter{ViewerID: q.ViewerID, CourseID: q.CourseID, After: after, Limit: q.Limit + 1}, nil
}

func (m Module) addComment(ctx context.Context, userID uuid.UUID, change commentChange) (*CourseComment, error) {
	content, err := cleanComment(change.Content)
	if err != nil {
		return nil, err
	}

	var row *commentRow
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.allowCreate(ctx, userID); err != nil {
			return err
		}
		row, err = m.comments.create(ctx, change.CourseID, userID, content)
		return err
	})
	if err != nil {
		return nil, err
	}
	comment := toComment(*row, userID)
	return &comment, nil
}

func (m Module) listComments(ctx context.Context, q commentQuery) ([]CourseComment, *string, error) {
	filter, err := q.filter()
	if err != nil {
		return nil, nil, err
	}
	if _, err := m.courses.get(ctx, q.ViewerID, q.CourseID); err != nil {
		return nil, nil, err
	}
	rows, err := m.comments.list(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	rows, next, err := httpapi.Page(rows, q.Limit, func(last commentRow) any {
		return pageCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	})
	if err != nil {
		return nil, nil, err
	}
	comments := make([]CourseComment, 0, len(rows))
	for _, row := range rows {
		comments = append(comments, toComment(row, q.ViewerID))
	}
	return comments, next, nil
}

func (m Module) deleteComment(ctx context.Context, userID, id uuid.UUID) error {
	return m.comments.delete(ctx, id, userID)
}

func toComment(row commentRow, viewerID uuid.UUID) CourseComment {
	return CourseComment{
		ID:        row.ID,
		CourseID:  row.CourseID,
		Content:   row.Content,
		IsMine:    row.AuthorID == viewerID,
		CreatedAt: row.CreatedAt,
	}
}
