package connections

import (
	"context"
	"errors"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
)

const (
	passCooldown      = 30 * 24 * time.Hour
	candidateLifetime = 30 * 24 * time.Hour
)

type discoveryQuery struct {
	ViewerID        uuid.UUID
	Mode            Mode
	Universities    []string
	Faculty         string
	Levels          []string
	Gender          string
	SharedInterests bool
	Limit           int
}

func (q discoveryQuery) filter() (discoveryFilter, error) {
	if !q.Mode.valid() {
		return discoveryFilter{}, ErrInvalidInput
	}
	f := discoveryFilter{
		ViewerID:        q.ViewerID,
		Mode:            q.Mode,
		Universities:    q.Universities,
		Gender:          q.Gender,
		SharedInterests: q.SharedInterests,
		PassCooldown:    passCooldown,
		Limit:           q.Limit,
	}
	if q.Mode == ModeStudyBuddy {
		f.Faculty = q.Faculty
		f.Levels = q.Levels
	}
	return f, nil
}

func (m Module) discover(ctx context.Context, q discoveryQuery) ([]Card, error) {
	f, err := q.filter()
	if err != nil {
		return nil, err
	}
	rows, err := m.discovery.find(ctx, f)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []Card{}, nil
	}

	userIDs := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		userIDs = append(userIDs, row.UserID)
	}
	candidates, err := m.discovery.candidates(ctx, q.ViewerID, userIDs)
	if err != nil {
		return nil, err
	}

	cards := make([]Card, 0, len(rows))
	for _, row := range rows {
		candidateID, ok := candidates[row.UserID]
		if !ok {
			continue
		}
		cards = append(cards, Card{
			CandidateID:     candidateID,
			Intent:          row.Intent,
			Bio:             row.Bio,
			Interests:       text.NonNil(row.Interests),
			SharedInterests: text.NonNil(row.SharedInterests),
			UniversityID:    row.UniversityID,
			Level:           row.Level,
			Age:             row.Age,
		})
	}
	return cards, nil
}

func (m Module) act(ctx context.Context, viewerID, candidateID uuid.UUID, mode Mode, a action) (*ActionResult, error) {
	if !mode.valid() || (a != actionLike && a != actionPass) {
		return nil, ErrInvalidInput
	}
	targetID, err := m.discovery.lookup(ctx, viewerID, candidateID)
	if err != nil {
		return nil, err
	}

	var match *connectionRow
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.discovery.lockPair(ctx, viewerID, targetID); err != nil {
			return err
		}
		ok, err := m.discovery.canMeet(ctx, viewerID, targetID, mode)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotFound
		}
		if a == actionLike {
			if err := m.likes.Allow(ctx, viewerID.String()); err != nil {
				if errors.Is(err, ratelimit.ErrLimited) {
					return errors.Join(ErrRateLimited, err)
				}
				return err
			}
		}
		if err := m.discovery.act(ctx, viewerID, targetID, mode, a); err != nil {
			return err
		}
		if a != actionLike {
			return nil
		}

		reciprocal, err := m.discovery.liked(ctx, targetID, viewerID, mode)
		if err != nil || !reciprocal {
			return err
		}
		match, err = m.connections.create(ctx, viewerID, targetID, mode)
		return err
	})
	if err != nil {
		return nil, err
	}

	result := &ActionResult{Matched: match != nil}
	if match == nil {
		return result, nil
	}
	connections, err := m.withProfiles(ctx, []connectionRow{*match})
	if err != nil {
		return nil, err
	}
	if len(connections) == 1 {
		result.Connection = &connections[0]
	}
	return result, nil
}
