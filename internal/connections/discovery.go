package connections

import (
	"context"
	"errors"

	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
)

const (
	actionLike = "like"
	actionPass = "pass"
)

type discoverQuery struct {
	ViewerID     uuid.UUID
	Mode         string
	Universities []string
	Faculty      string
	Levels       []string
	Gender       string
	Shared       bool
	Limit        int
}

func (m Module) discover(ctx context.Context, q discoverQuery) ([]Card, error) {
	if !validMode(q.Mode) {
		return nil, ErrInvalidInput
	}
	if q.Mode != modeStudy {
		q.Faculty = ""
		q.Levels = nil
	}

	rows, err := m.discovery.find(ctx, q)
	if err != nil || len(rows) == 0 {
		return []Card{}, err
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

func (m Module) act(ctx context.Context, viewerID, candidateID uuid.UUID, mode, action string) (*ActionResult, error) {
	if !validMode(mode) || (action != actionLike && action != actionPass) {
		return nil, ErrInvalidInput
	}
	targetID, err := m.discovery.lookup(ctx, viewerID, candidateID)
	if err != nil {
		return nil, err
	}

	var match *connectionRow
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		eligible, err := m.discovery.lockPair(ctx, viewerID, targetID, mode)
		if err != nil {
			return err
		}
		if !eligible {
			return ErrNotFound
		}
		if action == actionLike {
			if err := m.likes.Allow(ctx, viewerID.String()); err != nil {
				if errors.Is(err, ratelimit.ErrLimited) {
					return errors.Join(ErrRateLimited, err)
				}
				return err
			}
		}
		if err := m.discovery.act(ctx, viewerID, targetID, mode, action); err != nil {
			return err
		}
		if action != actionLike {
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
