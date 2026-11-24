package feed

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var errStillActive = errors.New("feed content of the user keeps appearing; revoke the user's sessions first")

const forgetPasses = 3

// ForgetUser must run after the user's sessions are revoked, outside any
// transaction, and before the user row is deleted.
func (m Module) ForgetUser(ctx context.Context, userID uuid.UUID) error {
	for range forgetPasses {
		found, err := m.forgetPass(ctx, userID)
		if err != nil || !found {
			return err
		}
	}
	return errStillActive
}

func (m Module) forgetPass(ctx context.Context, userID uuid.UUID) (bool, error) {
	postIDs, err := m.posts.idsByAuthor(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, id := range postIDs {
		if err := ignoreGone(m.removePost(ctx, id, nil)); err != nil {
			return false, err
		}
	}

	commentIDs, err := m.comments.idsByAuthor(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, id := range commentIDs {
		if err := ignoreGone(m.removeComment(ctx, id, nil)); err != nil {
			return false, err
		}
	}

	unsaved, err := m.posts.unsaveAll(ctx, userID)
	if err != nil {
		return false, err
	}

	likedPosts, err := m.likes.postIDsLikedBy(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, id := range likedPosts {
		if _, _, err := m.setPostLike(ctx, userID, id, false); ignoreGone(err) != nil {
			return false, err
		}
	}

	likedComments, err := m.likes.commentIDsLikedBy(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, id := range likedComments {
		if _, err := m.setCommentLike(ctx, userID, id, false); ignoreGone(err) != nil {
			return false, err
		}
	}
	return len(postIDs)+len(commentIDs)+int(unsaved)+len(likedPosts)+len(likedComments) > 0, nil
}

func ignoreGone(err error) error {
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
