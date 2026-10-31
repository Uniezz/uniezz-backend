package feed

import (
	"context"
	"errors"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

func allowAny(uuid.UUID) error { return nil }

var errStillActive = errors.New("feed content of the user keeps appearing; revoke the user's sessions first")

const forgetPasses = 3

// ForgetUser removes everything the user left in the feed through the same
// per-item paths a user takes, so counters, threads and media stay consistent:
// own posts go with their comments and media, own root comments that others
// answered stay as anonymous tombstones, and likes are taken back. Each item
// commits on its own, so a failed run can simply be repeated, and it must not
// run inside a caller's transaction: media leaves storage after each commit.
// Call it after the user's sessions are revoked and before the user row is
// deleted; the feed's foreign keys to users refuse the delete while anything
// is left.
func (m Module) ForgetUser(ctx context.Context, userID uuid.UUID) error {
	if database.InTx(ctx) {
		return errors.New("feed: ForgetUser must not run inside a transaction")
	}
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
		if err := ignoreGone(m.removePost(ctx, id, allowAny)); err != nil {
			return false, err
		}
	}

	commentIDs, err := m.comments.idsByAuthor(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, id := range commentIDs {
		if err := ignoreGone(m.removeComment(ctx, id, allowAny)); err != nil {
			return false, err
		}
	}

	likedPosts, err := m.likes.postIDsLikedBy(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, id := range likedPosts {
		if _, err := m.setPostLike(ctx, userID, id, false); ignoreGone(err) != nil {
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
	return len(postIDs)+len(commentIDs)+len(likedPosts)+len(likedComments) > 0, nil
}

func ignoreGone(err error) error {
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
