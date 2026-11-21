package rewards

import (
	"context"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/google/uuid"
)

type Action string

const (
	ActionLikeGiven        Action = "like_given"
	ActionLikeReceived     Action = "like_received"
	ActionCommentPosted    Action = "comment_posted"
	ActionReplyPosted      Action = "reply_posted"
	ActionPostCreated      Action = "post_created"
	ActionFileUploaded     Action = "file_uploaded"
	ActionCourseRated      Action = "course_rated"
	ActionProfessorRated   Action = "professor_rated"
	ActionConnectionMade   Action = "connection_made"
	ActionProfileCompleted Action = "profile_completed"
	ActionFriendInvited    Action = "friend_invited"
	ActionFAQApproved      Action = "faq_approved"
)

type Period string

const (
	PeriodDay  Period = "day"
	PeriodWeek Period = "week"
	PeriodOnce Period = "once"
)

type challengeChange struct {
	Action   Action
	Target   int
	Period   Period
	Reward   int
	Texts    map[string]ChallengeText
	StartsAt *time.Time
	EndsAt   *time.Time
}

type challengeEdit struct {
	Texts    *map[string]ChallengeText
	Reward   *int
	StartsAt *time.Time
	EndsAt   *time.Time
}

type challengeQuery struct {
	Cursor string
	Limit  int
}

func (c challengeChange) fields() (challengeFields, error) {
	texts, err := cleanTexts(c.Texts)
	if err != nil {
		return challengeFields{}, err
	}
	if c.StartsAt != nil && c.EndsAt != nil && !c.EndsAt.After(*c.StartsAt) {
		return challengeFields{}, ErrInvalidInput
	}
	return challengeFields{
		Action: c.Action, Target: c.Target, Period: c.Period, Reward: c.Reward,
		Texts: texts, StartsAt: c.StartsAt, EndsAt: c.EndsAt,
	}, nil
}

func (m Module) createChallenge(ctx context.Context, change challengeChange) (*Challenge, error) {
	f, err := change.fields()
	if err != nil {
		return nil, err
	}
	row, err := m.challenges.create(ctx, f)
	if err != nil {
		return nil, err
	}
	challenge := toChallenge(*row)
	return &challenge, nil
}

func (m Module) challenge(ctx context.Context, id uuid.UUID) (*Challenge, error) {
	row, err := m.challenges.get(ctx, id)
	if err != nil {
		return nil, err
	}
	challenge := toChallenge(*row)
	return &challenge, nil
}

func (m Module) updateChallenge(ctx context.Context, id uuid.UUID, edit challengeEdit) (*Challenge, error) {
	u := challengeUpdate{Reward: edit.Reward, StartsAt: edit.StartsAt, EndsAt: edit.EndsAt}
	if edit.Texts != nil {
		texts, err := cleanTexts(*edit.Texts)
		if err != nil {
			return nil, err
		}
		u.Texts = texts
	}
	if edit.StartsAt != nil && edit.EndsAt != nil && !edit.EndsAt.After(*edit.StartsAt) {
		return nil, ErrInvalidInput
	}
	row, err := m.challenges.update(ctx, id, u)
	if err != nil {
		return nil, err
	}
	challenge := toChallenge(*row)
	return &challenge, nil
}

func (m Module) archiveChallenge(ctx context.Context, id uuid.UUID) error {
	return m.challenges.archive(ctx, id)
}

func (m Module) listChallenges(ctx context.Context, q challengeQuery) ([]Challenge, *string, error) {
	filter, err := challengePage(q)
	if err != nil {
		return nil, nil, err
	}
	rows, err := m.challenges.list(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	rows, next, err := httpapi.Page(rows, q.Limit, func(last challengeRow) any {
		return challengeCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	})
	if err != nil {
		return nil, nil, err
	}
	items := make([]Challenge, 0, len(rows))
	for _, row := range rows {
		items = append(items, toChallenge(row))
	}
	return items, next, nil
}

func (m Module) myChallenges(ctx context.Context, userID uuid.UUID, q challengeQuery) ([]MyChallenge, *string, error) {
	filter, err := challengePage(q)
	if err != nil {
		return nil, nil, err
	}
	filter.UserID = userID
	rows, err := m.challenges.listMine(ctx, questZone, filter)
	if err != nil {
		return nil, nil, err
	}
	rows, next, err := httpapi.Page(rows, q.Limit, func(last myChallengeRow) any {
		return challengeCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	})
	if err != nil {
		return nil, nil, err
	}
	items := make([]MyChallenge, 0, len(rows))
	for _, row := range rows {
		items = append(items, MyChallenge{
			ID: row.ID, Action: row.Action, Target: row.Target, Period: row.Period, Reward: row.Reward,
			Texts: row.Texts, EndsAt: row.EndsAt,
			Progress: min(row.Progress, row.Target), Completed: row.Completed, ResetsAt: row.ResetsAt,
		})
	}
	return items, next, nil
}

func challengePage(q challengeQuery) (challengeFilter, error) {
	filter := challengeFilter{Limit: q.Limit + 1}
	if q.Cursor != "" {
		var after challengeCursor
		if err := httpapi.DecodeCursor(q.Cursor, &after); err != nil {
			return challengeFilter{}, err
		}
		filter.After = &after
	}
	return filter, nil
}

func toChallenge(row challengeRow) Challenge {
	return Challenge{
		ID: row.ID, Action: row.Action, Target: row.Target, Period: row.Period, Reward: row.Reward,
		Texts: row.Texts, StartsAt: row.StartsAt, EndsAt: row.EndsAt, ArchivedAt: row.ArchivedAt, CreatedAt: row.CreatedAt,
	}
}
