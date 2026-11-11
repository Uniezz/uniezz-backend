package feed

import (
	"context"
	"time"

	"github.com/google/uuid"
)

func (m Module) setPostLike(ctx context.Context, userID, postID uuid.UUID, liked bool) (*LikeState, error) {
	var state *LikeState
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		if _, err := m.posts.lock(ctx, postID, false); err != nil {
			return err
		}
		if liked {
			if err := m.posts.visible(ctx, userID, postID); err != nil {
				return err
			}
		}
		var err error
		var change *likeChange
		state, change, err = m.setLike(ctx, postLikes, postID, userID, liked)
		if err != nil || change == nil {
			return err
		}
		return m.likes.countWeekly(ctx, postID, change.likedAt, change.delta)
	})
	return state, err
}

func (m Module) setCommentLike(ctx context.Context, userID, commentID uuid.UUID, liked bool) (*LikeState, error) {
	var state *LikeState
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.comments.lockLive(ctx, commentID); err != nil {
			return err
		}
		if liked {
			if err := m.comments.visible(ctx, userID, commentID); err != nil {
				return err
			}
		}
		var err error
		state, _, err = m.setLike(ctx, commentLikes, commentID, userID, liked)
		return err
	})
	return state, err
}

type likeChange struct {
	likedAt time.Time
	delta   int
}

// setLike runs with the liked row already locked, so the counter changes
// exactly when a like row was inserted or deleted. The change is nil when
// nothing changed.
func (m Module) setLike(ctx context.Context, t likeTarget, targetID, userID uuid.UUID, liked bool) (*LikeState, *likeChange, error) {
	var likedAt *time.Time
	var err error
	delta := 1
	if liked {
		likedAt, err = m.likes.add(ctx, t, targetID, userID)
	} else {
		likedAt, err = m.likes.remove(ctx, t, targetID, userID)
		delta = -1
	}
	if err != nil {
		return nil, nil, err
	}
	if likedAt == nil {
		count, err := m.likes.current(ctx, t, targetID)
		if err != nil {
			return nil, nil, err
		}
		return &LikeState{Liked: liked, LikesCount: count}, nil, nil
	}
	count, err := m.likes.count(ctx, t, targetID, delta)
	if err != nil {
		return nil, nil, err
	}
	return &LikeState{Liked: liked, LikesCount: count}, &likeChange{likedAt: *likedAt, delta: delta}, nil
}
