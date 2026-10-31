package feed

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/Uniezz/uniezz-backend/internal/telemetry"
	"github.com/google/uuid"
)

const (
	allUniversities = "all"
	typeEvent       = "event"
	maxPostMedia    = 10
)

type postInput struct {
	Type       string      `json:"type"`
	University string      `json:"university"`
	Title      string      `json:"title"`
	Content    string      `json:"content"`
	Tags       []string    `json:"tags"`
	EventDate  *time.Time  `json:"eventDate"`
	MediaIDs   []uuid.UUID `json:"mediaIds"`
}

func (in postInput) clean() (postInput, error) {
	var err error
	if in.Title, err = cleanText(in.Title, false, minTitleLength, maxTitleLength); err != nil {
		return postInput{}, err
	}
	if in.Content, err = cleanText(in.Content, true, minContentLength, maxContentLength); err != nil {
		return postInput{}, err
	}
	if in.Tags, err = cleanTags(in.Tags); err != nil {
		return postInput{}, err
	}
	if in.EventDate != nil && in.Type != typeEvent {
		return postInput{}, ErrInvalidInput
	}
	if in.EventDate != nil {
		utc := in.EventDate.UTC()
		in.EventDate = &utc
	}
	if len(in.MediaIDs) > maxPostMedia || hasDuplicates(in.MediaIDs) {
		return postInput{}, ErrInvalidInput
	}
	if in.MediaIDs == nil {
		in.MediaIDs = []uuid.UUID{}
	}
	return in, nil
}

// sortedIDs gives media row locks one order, so two requests attaching the
// same media cannot deadlock.
func sortedIDs(ids []uuid.UUID) []uuid.UUID {
	sorted := slices.Clone(ids)
	slices.SortFunc(sorted, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	return sorted
}

func hasDuplicates(ids []uuid.UUID) bool {
	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return true
		}
		seen[id] = true
	}
	return false
}

// createPost returns the post and whether it was created by an earlier request
// with the same idempotency key.
func (m Module) createPost(ctx context.Context, userID uuid.UUID, in postInput, key string) (*Post, bool, error) {
	in, err := in.clean()
	if err != nil {
		return nil, false, err
	}
	fp, err := fingerprint(in)
	if err != nil {
		return nil, false, err
	}

	if key != "" {
		post, err := m.replayPost(ctx, userID, key, fp)
		if !errors.Is(err, ErrNotFound) {
			return post, err == nil, err
		}
	}

	if err := m.checkUniversity(ctx, userID, in.University); err != nil {
		return nil, false, err
	}
	for _, id := range in.MediaIDs {
		if err := m.media.CheckUpload(ctx, id, userID, media.KindFeed); err != nil {
			return nil, false, mediaError(err)
		}
	}

	postID := uuid.New()
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.claimKey(ctx, userID, opCreatePost, key, fp, postID); err != nil {
			return err
		}
		err := m.posts.insert(ctx, newPost{
			ID: postID, AuthorID: userID, UniversityID: in.University, Type: in.Type,
			Title: in.Title, Content: in.Content, EventDate: in.EventDate, Tags: in.Tags,
		})
		if err != nil {
			return err
		}
		for _, id := range sortedIDs(in.MediaIDs) {
			if err := m.media.MarkReady(ctx, id, userID, media.KindFeed); err != nil {
				return mediaError(err)
			}
		}
		for i, id := range in.MediaIDs {
			if err := m.posts.attachMedia(ctx, postID, i, id); err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, errKeyTaken) {
		post, err := m.replayPost(ctx, userID, key, fp)
		return post, err == nil, err
	}
	if err != nil {
		return nil, false, err
	}

	post, err := m.post(ctx, userID, postID)
	return post, false, err
}

func (m Module) replayPost(ctx context.Context, userID uuid.UUID, key string, fp []byte) (*Post, error) {
	postID, err := m.replayed(ctx, userID, opCreatePost, key, fp)
	if err != nil {
		return nil, err
	}
	post, err := m.post(ctx, userID, postID)
	return post, resourceGone(err)
}

func (m Module) checkUniversity(ctx context.Context, userID uuid.UUID, university string) error {
	if university == allUniversities {
		return nil
	}
	own, err := m.posts.authorUniversity(ctx, userID)
	if err != nil {
		return err
	}
	if university != own {
		return ErrInvalidUniversity
	}
	return nil
}

func mediaError(err error) error {
	if errors.Is(err, media.ErrNotFound) || errors.Is(err, media.ErrUploadMismatch) {
		return errors.Join(ErrInvalidMedia, err)
	}
	return err
}

func (m Module) post(ctx context.Context, viewerID, postID uuid.UUID) (*Post, error) {
	row, err := m.posts.get(ctx, viewerID, postID)
	if err != nil {
		return nil, err
	}
	posts, err := m.toPosts(ctx, []postRow{*row})
	if err != nil {
		return nil, err
	}
	return &posts[0], nil
}

type postQuery struct {
	ViewerID   uuid.UUID
	University string
	Type       string
	From       *time.Time
	To         *time.Time
	Cursor     string
	Limit      int
}

func (m Module) listPosts(ctx context.Context, q postQuery) ([]Post, *string, error) {
	if q.University != "" && q.University != allUniversities {
		if _, err := auth.GetUniversityByID(auth.UniversityID(q.University)); err != nil {
			return nil, nil, ErrInvalidInput
		}
	}
	if q.From != nil && q.To != nil && !q.From.Before(*q.To) {
		return nil, nil, ErrInvalidInput
	}

	filter := postFilter{ViewerID: q.ViewerID, UniversityID: q.University, Type: q.Type, From: q.From, To: q.To, Limit: q.Limit + 1}
	if q.Cursor != "" {
		var after postCursor
		if err := httpapi.DecodeCursor(q.Cursor, &after); err != nil {
			return nil, nil, err
		}
		filter.After = &after
	}

	rows, err := m.posts.list(ctx, filter)
	if err != nil {
		return nil, nil, err
	}

	var next *string
	if len(rows) > q.Limit {
		rows = rows[:q.Limit]
		last := rows[len(rows)-1]
		cursor, err := httpapi.EncodeCursor(postCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return nil, nil, err
		}
		next = &cursor
	}

	posts, err := m.toPosts(ctx, rows)
	if err != nil {
		return nil, nil, err
	}
	return posts, next, nil
}

func (m Module) toPosts(ctx context.Context, rows []postRow) ([]Post, error) {
	ids := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	refs, err := m.posts.mediaOf(ctx, ids)
	if err != nil {
		return nil, err
	}

	posts := make([]Post, 0, len(rows))
	for _, row := range rows {
		urls := make([]string, 0, len(refs[row.ID]))
		for _, ref := range refs[row.ID] {
			if url := m.mediaURL(ctx, &ref); url != nil {
				urls = append(urls, *url)
			}
		}
		posts = append(posts, Post{
			ID:            row.ID,
			Type:          row.Type,
			University:    row.UniversityID,
			Title:         row.Title,
			Content:       row.Content,
			Tags:          nonNil(row.Tags),
			EventDate:     row.EventDate,
			MediaURLs:     urls,
			Author:        m.toAuthor(ctx, row.Author),
			LikesCount:    row.LikesCount,
			CommentsCount: row.CommentsCount,
			LikedByMe:     row.LikedByMe,
			CreatedAt:     row.CreatedAt,
			EditedAt:      row.EditedAt,
		})
	}
	return posts, nil
}

func (m Module) toAuthor(ctx context.Context, a authorRow) Author {
	return Author{
		ID:           a.ID,
		FirstName:    a.FirstName,
		LastName:     a.LastName,
		UniversityID: a.UniversityID,
		AvatarURL:    m.mediaURL(ctx, a.Avatar),
	}
}

func (m Module) mediaURL(ctx context.Context, ref *media.Ref) *string {
	if ref == nil {
		return nil
	}
	url, err := m.media.URLOf(ctx, *ref)
	if err != nil {
		telemetry.ReportError(ctx, "resolve feed media URL", err, slog.String("object_key", ref.ObjectKey))
		return nil
	}
	return &url
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

type postEdit struct {
	Title     *string
	Content   *string
	EventDate *time.Time
	Tags      *[]string
}

func (e postEdit) clean() (postChange, error) {
	var c postChange
	if e.Title != nil {
		title, err := cleanText(*e.Title, false, minTitleLength, maxTitleLength)
		if err != nil {
			return postChange{}, err
		}
		c.Title = &title
	}
	if e.Content != nil {
		content, err := cleanText(*e.Content, true, minContentLength, maxContentLength)
		if err != nil {
			return postChange{}, err
		}
		c.Content = &content
	}
	if e.Tags != nil {
		tags, err := cleanTags(*e.Tags)
		if err != nil {
			return postChange{}, err
		}
		c.Tags = tags
	}
	if e.EventDate != nil {
		utc := e.EventDate.UTC()
		c.EventDate = &utc
	}
	return c, nil
}

func (m Module) updatePost(ctx context.Context, userID, postID uuid.UUID, edit postEdit) (*Post, error) {
	change, err := edit.clean()
	if err != nil {
		return nil, err
	}

	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		locked, err := m.posts.lock(ctx, postID, false)
		if err != nil {
			return err
		}
		if locked.AuthorID != userID {
			return ErrForbidden
		}
		if change.EventDate != nil && locked.Type != typeEvent {
			return ErrInvalidInput
		}
		if change.empty() {
			return nil
		}
		return m.posts.update(ctx, postID, change)
	})
	if err != nil {
		return nil, err
	}
	return m.post(ctx, userID, postID)
}

func (m Module) deletePost(ctx context.Context, userID, postID uuid.UUID) error {
	return m.removePost(ctx, postID, func(authorID uuid.UUID) error {
		if authorID != userID {
			return ErrForbidden
		}
		return nil
	})
}

// removePost deletes the post with its comments and likes, and hands its media
// to the media module: marked deleting in the same transaction, deleted from
// storage after the commit. A failed storage delete is retried by media cleanup.
func (m Module) removePost(ctx context.Context, postID uuid.UUID, allow func(authorID uuid.UUID) error) error {
	var mediaIDs []uuid.UUID
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		locked, err := m.posts.lock(ctx, postID, true)
		if err != nil {
			return err
		}
		if err := allow(locked.AuthorID); err != nil {
			return err
		}
		ids, err := m.posts.mediaIDs(ctx, postID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			err := m.media.MarkDeleting(ctx, id)
			if errors.Is(err, media.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			mediaIDs = append(mediaIDs, id)
		}
		return m.posts.delete(ctx, postID)
	})
	if err != nil {
		return err
	}

	for _, id := range mediaIDs {
		if err := m.media.Delete(ctx, id); err != nil {
			slog.WarnContext(ctx, "delete post media, left for cleanup", slog.String("media_id", id.String()), slog.Any("error", err))
		}
	}
	return nil
}
