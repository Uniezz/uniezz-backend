// Package feed owns the community feed: posts, comments on them and likes.
// Every write that changes a counter locks rows top-down — post, then root
// comment, then reply — and changes the counter in the same transaction, only
// when a row was really inserted or deleted.
package feed

import (
	"context"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ mediaService = media.Module{}

type mediaService interface {
	CheckUpload(ctx context.Context, mediaID, ownerUserID uuid.UUID, kind media.Kind) error
	MarkReady(ctx context.Context, mediaID, ownerUserID uuid.UUID, kind media.Kind) error
	URLOf(ctx context.Context, ref media.Ref) (string, error)
	MarkDeleting(ctx context.Context, mediaID uuid.UUID) error
	Delete(ctx context.Context, mediaID uuid.UUID) error
}

type Module struct {
	tx       database.Transactor
	posts    *postRepository
	comments *commentRepository
	likes    *likeRepository
	keys     *requestKeyRepository
	media    mediaService
	jobs     *jobs.Runner
}

func New(pool *pgxpool.Pool, mediaModule mediaService, runner *jobs.Runner) (Module, error) {
	db := database.New(pool)
	m := Module{
		tx:       db,
		posts:    newPostRepository(db),
		comments: newCommentRepository(db),
		likes:    newLikeRepository(db),
		keys:     newRequestKeyRepository(db),
		media:    mediaModule,
		jobs:     runner,
	}

	if err := jobs.Register(runner, m.pruneKeys, jobs.Options{Timeout: time.Minute, MaxAttempts: 3}); err != nil {
		return Module{}, fmt.Errorf("feed: %w", err)
	}
	if err := jobs.Schedule(runner, PruneKeysArgs{}, 24*time.Hour); err != nil {
		return Module{}, fmt.Errorf("feed: %w", err)
	}
	return m, nil
}

func (m Module) Register(api huma.API) {
	posts := &postHandler{feed: m}
	comments := &commentHandler{feed: m}
	likes := &likeHandler{feed: m}

	huma.Register(api, createPostOperation, posts.createPost)
	huma.Register(api, listPostsOperation, posts.listPosts)
	huma.Register(api, getPostOperation, posts.getPost)
	huma.Register(api, updatePostOperation, posts.updatePost)
	huma.Register(api, deletePostOperation, posts.deletePost)
	huma.Register(api, likePostOperation, likes.likePost)
	huma.Register(api, unlikePostOperation, likes.unlikePost)
	huma.Register(api, createCommentOperation, comments.createComment)
	huma.Register(api, listCommentsOperation, comments.listComments)
	huma.Register(api, listRepliesOperation, comments.listReplies)
	huma.Register(api, updateCommentOperation, comments.updateComment)
	huma.Register(api, deleteCommentOperation, comments.deleteComment)
	huma.Register(api, likeCommentOperation, likes.likeComment)
	huma.Register(api, unlikeCommentOperation, likes.unlikeComment)
}
