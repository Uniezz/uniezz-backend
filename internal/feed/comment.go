package feed

import (
	"context"
	"errors"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/google/uuid"
)

type commentInput struct {
	PostID           uuid.UUID  `json:"postId"`
	ReplyToCommentID *uuid.UUID `json:"replyToCommentId"`
	Content          string     `json:"content"`
}

// createComment adds a root comment, or a reply when ReplyToCommentID is set.
// A reply to a reply joins the same thread: its parent is the thread's root.
func (m Module) createComment(ctx context.Context, userID uuid.UUID, in commentInput, key string) (*Comment, bool, error) {
	content, err := cleanText(in.Content, true, 1, maxCommentLength)
	if err != nil {
		return nil, false, err
	}
	in.Content = content
	fp, err := fingerprint(in)
	if err != nil {
		return nil, false, err
	}

	if key != "" {
		comment, err := m.replayComment(ctx, userID, key, fp)
		if !errors.Is(err, ErrNotFound) {
			return comment, err == nil, err
		}
	}

	var parentID *uuid.UUID
	if in.ReplyToCommentID != nil {
		target, err := m.comments.ref(ctx, *in.ReplyToCommentID)
		if err != nil {
			return nil, false, err
		}
		if target.PostID != in.PostID {
			return nil, false, ErrNotFound
		}
		parentID = &target.ThreadID
	}

	commentID := uuid.New()
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.claimKey(ctx, userID, opCreateComment, key, fp, commentID); err != nil {
			return err
		}
		if err := m.posts.countComments(ctx, in.PostID, 1); err != nil {
			return err
		}
		if parentID != nil {
			if err := m.comments.addReply(ctx, in.PostID, *parentID); err != nil {
				return err
			}
			if err := m.comments.liveInThread(ctx, *in.ReplyToCommentID, *parentID); err != nil {
				return err
			}
		}
		return m.comments.insert(ctx, newComment{
			ID: commentID, PostID: in.PostID, AuthorID: userID,
			ParentID: parentID, ReplyToCommentID: in.ReplyToCommentID, Content: in.Content,
		})
	})
	if errors.Is(err, errKeyTaken) {
		comment, err := m.replayComment(ctx, userID, key, fp)
		return comment, err == nil, err
	}
	if err != nil {
		return nil, false, err
	}

	comment, err := m.comment(ctx, userID, commentID)
	return comment, false, err
}

func (m Module) replayComment(ctx context.Context, userID uuid.UUID, key string, fp []byte) (*Comment, error) {
	commentID, err := m.replayed(ctx, userID, opCreateComment, key, fp)
	if err != nil {
		return nil, err
	}
	comment, err := m.comment(ctx, userID, commentID)
	if err == nil && comment.Deleted {
		return nil, ErrResourceDeleted
	}
	return comment, resourceGone(err)
}

func (m Module) comment(ctx context.Context, viewerID, commentID uuid.UUID) (*Comment, error) {
	row, err := m.comments.get(ctx, viewerID, commentID)
	if err != nil {
		return nil, err
	}
	c := m.toComment(ctx, *row)
	return &c, nil
}

func (m Module) listComments(ctx context.Context, viewerID, postID uuid.UUID, cursor string, limit int) ([]Comment, *string, error) {
	if err := m.posts.exists(ctx, postID); err != nil {
		return nil, nil, err
	}
	return m.commentPage(ctx, cursor, limit, func(after *pageCursor) ([]commentRow, error) {
		return m.comments.listRoots(ctx, viewerID, postID, after, limit+1)
	})
}

func (m Module) listReplies(ctx context.Context, viewerID, rootID uuid.UUID, cursor string, limit int) ([]Comment, *string, error) {
	if err := m.comments.isRoot(ctx, rootID); err != nil {
		return nil, nil, err
	}
	return m.commentPage(ctx, cursor, limit, func(after *pageCursor) ([]commentRow, error) {
		return m.comments.listReplies(ctx, viewerID, rootID, after, limit+1)
	})
}

func (m Module) commentPage(ctx context.Context, cursor string, limit int, list func(after *pageCursor) ([]commentRow, error)) ([]Comment, *string, error) {
	var after *pageCursor
	if cursor != "" {
		after = &pageCursor{}
		if err := httpapi.DecodeCursor(cursor, after); err != nil {
			return nil, nil, err
		}
	}

	rows, err := list(after)
	if err != nil {
		return nil, nil, err
	}

	var next *string
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		encoded, err := httpapi.EncodeCursor(pageCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return nil, nil, err
		}
		next = &encoded
	}

	comments := make([]Comment, 0, len(rows))
	for _, row := range rows {
		comments = append(comments, m.toComment(ctx, row))
	}
	return comments, next, nil
}

func (m Module) toComment(ctx context.Context, row commentRow) Comment {
	c := Comment{
		ID:               row.ID,
		PostID:           row.PostID,
		ParentID:         row.ParentID,
		ReplyToCommentID: row.ReplyToCommentID,
		Content:          row.Content,
		Deleted:          row.Deleted,
		LikesCount:       row.LikesCount,
		RepliesCount:     row.RepliesCount,
		LikedByMe:        row.LikedByMe,
		CreatedAt:        row.CreatedAt,
		EditedAt:         row.EditedAt,
	}
	if row.Author != nil {
		author := m.toAuthor(ctx, *row.Author)
		c.Author = &author
	}
	return c
}

func (m Module) updateComment(ctx context.Context, userID, commentID uuid.UUID, content string) (*Comment, error) {
	content, err := cleanText(content, true, 1, maxCommentLength)
	if err != nil {
		return nil, err
	}

	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		locked, err := m.comments.lockForChange(ctx, commentID, false)
		if err != nil {
			return err
		}
		if err := onlyAuthor(userID)(locked.AuthorID); err != nil {
			return err
		}
		return m.comments.update(ctx, commentID, content)
	})
	if err != nil {
		return nil, err
	}
	return m.comment(ctx, userID, commentID)
}

func (m Module) deleteComment(ctx context.Context, userID, commentID uuid.UUID) error {
	return m.removeComment(ctx, commentID, onlyAuthor(userID))
}

// removeComment locks top-down — post, root, the comment — then deletes the
// comment. A root that still has replies becomes a tombstone so the thread
// survives; the post loses one live comment either way, exactly once.
func (m Module) removeComment(ctx context.Context, commentID uuid.UUID, allow func(authorID uuid.UUID) error) error {
	ref, err := m.comments.ref(ctx, commentID)
	if err != nil {
		return err
	}

	return m.tx.WithTx(ctx, func(ctx context.Context) error {
		if _, err := m.posts.lock(ctx, ref.PostID, false); err != nil {
			return err
		}
		if ref.ParentID != nil {
			if err := m.comments.lockRoot(ctx, *ref.ParentID); err != nil {
				return err
			}
		}
		locked, err := m.comments.lockForChange(ctx, commentID, true)
		if err != nil {
			return err
		}
		if err := allow(locked.AuthorID); err != nil {
			return err
		}
		if err := m.posts.countComments(ctx, ref.PostID, -1); err != nil {
			return err
		}

		if ref.ParentID == nil {
			if locked.RepliesCount > 0 {
				return m.comments.tombstone(ctx, commentID)
			}
			return m.comments.delete(ctx, commentID)
		}

		if err := m.comments.delete(ctx, commentID); err != nil {
			return err
		}
		orphan, err := m.comments.removeReply(ctx, *ref.ParentID)
		if err != nil {
			return err
		}
		if orphan {
			return m.comments.delete(ctx, *ref.ParentID)
		}
		return nil
	})
}
