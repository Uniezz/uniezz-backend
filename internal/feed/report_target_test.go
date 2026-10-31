package feed

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestReportTarget(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	post := e.createPost(t, author, nil)
	c := e.comment(t, author, post.ID, nil, "Reported comment")
	ctx := context.Background()

	authorID, snapshot, err := e.module.ReportTarget(ctx, TargetPost, post.ID)
	if err != nil || authorID != author.id || snapshot["title"] != post.Title || snapshot["content"] != post.Content {
		t.Errorf("post target = %s %v %v", authorID, snapshot, err)
	}

	authorID, snapshot, err = e.module.ReportTarget(ctx, TargetComment, c.ID)
	if err != nil || authorID != author.id || snapshot["content"] != "Reported comment" || snapshot["postId"] != post.ID {
		t.Errorf("comment target = %s %v %v", authorID, snapshot, err)
	}

	for name, call := range map[string]func() error{
		"missing post": func() error { _, _, err := e.module.ReportTarget(ctx, TargetPost, uuid.New()); return err },
		"unknown type": func() error { _, _, err := e.module.ReportTarget(ctx, "user", post.ID); return err },
		"deleted comment": func() error {
			e.api.Delete("/feed/comments/"+c.ID.String(), author.bearer)
			_, _, err := e.module.ReportTarget(ctx, TargetComment, c.ID)
			return err
		},
	} {
		if err := call(); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: error = %v, want ErrNotFound", name, err)
		}
	}
}
