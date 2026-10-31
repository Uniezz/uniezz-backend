package feed

import (
	"context"

	"github.com/google/uuid"
)

func (m Module) setPostLike(ctx context.Context, userID, postID uuid.UUID, liked bool) (*LikeState, error) {
	var state *LikeState
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		if _, err := m.posts.lock(ctx, postID, false); err != nil {
			return err
		}
		var err error
		state, err = m.setLike(ctx, postLikes, postID, userID, liked)
		return err
	})
	return state, err
}

func (m Module) setCommentLike(ctx context.Context, userID, commentID uuid.UUID, liked bool) (*LikeState, error) {
	var state *LikeState
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.comments.lockLive(ctx, commentID); err != nil {
			return err
		}
		var err error
		state, err = m.setLike(ctx, commentLikes, commentID, userID, liked)
		return err
	})
	return state, err
}

// setLike runs with the liked row already locked, so the counter changes
// exactly when a like row was inserted or deleted.
func (m Module) setLike(ctx context.Context, t likeTarget, targetID, userID uuid.UUID, liked bool) (*LikeState, error) {
	var changed bool
	var err error
	delta := 1
	if liked {
		changed, err = m.likes.add(ctx, t, targetID, userID)
	} else {
		changed, err = m.likes.remove(ctx, t, targetID, userID)
		delta = -1
	}
	if err != nil {
		return nil, err
	}
	var count int
	if changed {
		count, err = m.likes.count(ctx, t, targetID, delta)
	} else {
		count, err = m.likes.current(ctx, t, targetID)
	}
	if err != nil {
		return nil, err
	}
	return &LikeState{Liked: liked, LikesCount: count}, nil
}
