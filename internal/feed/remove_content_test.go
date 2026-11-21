package feed

import (
	"context"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/google/uuid"
)

func TestRemoveContent(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t.Run("removing another student's post takes its comments and media, and a repeat is a no-op", func(t *testing.T) {
		author, other := e.user(t), e.user(t)
		mediaID := e.insertMedia(t, author.id, media.KindFeed)
		post := e.createPost(t, author, map[string]any{"mediaIds": []uuid.UUID{mediaID}})
		e.comment(t, other, post.ID, nil, "Comment on a removed post")

		if err := e.module.RemovePost(ctx, post.ID); err != nil {
			t.Fatalf("RemovePost: %v", err)
		}
		if err := e.module.RemovePost(ctx, post.ID); err != nil {
			t.Fatalf("second RemovePost must be a no-op: %v", err)
		}

		var left int
		if err := e.pool.QueryRow(ctx, `
			SELECT (SELECT count(*) FROM feed_posts WHERE id = $1) + (SELECT count(*) FROM feed_comments WHERE post_id = $1)`,
			post.ID).Scan(&left); err != nil {
			t.Fatal(err)
		}
		if left != 0 {
			t.Errorf("%d rows of the post are left", left)
		}
		if got := e.mediaStatus(t, mediaID); got != "gone" {
			t.Errorf("post media = %q, want deleted", got)
		}
	})

	t.Run("removing another student's comment keeps the post's counter right", func(t *testing.T) {
		author, commenter := e.user(t), e.user(t)
		post := e.createPost(t, author, nil)
		comment := e.comment(t, commenter, post.ID, nil, "Rude comment")

		if err := e.module.RemoveComment(ctx, comment.ID); err != nil {
			t.Fatalf("RemoveComment: %v", err)
		}
		if err := e.module.RemoveComment(ctx, comment.ID); err != nil {
			t.Fatalf("second RemoveComment must be a no-op: %v", err)
		}
		if got := e.commentsCount(t, post.ID); got != 0 {
			t.Errorf("comments count = %d, want 0", got)
		}
		e.assertCounters(t, post.ID)
	})
}
