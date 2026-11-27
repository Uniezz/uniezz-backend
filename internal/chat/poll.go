package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
)

const (
	maxPollQuestion = 300
	maxPollOption   = 100
	minPollOptions  = 2
	maxPollOptions  = 10
	maxPollDuration = 30 * 24 * time.Hour
	shownVoters     = 5
)

type pollChange struct {
	Question       string
	Options        []string
	MultipleChoice bool
	Anonymous      bool
	ClosesAt       *time.Time
}

type pollOptionPayload struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

type pollPayload struct {
	Question       string              `json:"question"`
	Options        []pollOptionPayload `json:"options"`
	MultipleChoice bool                `json:"multipleChoice"`
	Anonymous      bool                `json:"anonymous"`
	ClosesAt       *time.Time          `json:"closesAt"`
	ClosedAt       *time.Time          `json:"closedAt"`
}

func (c pollChange) payload(now time.Time) (pollPayload, error) {
	question := text.Plain(c.Question, true)
	if n := utf8.RuneCountInString(question); n == 0 || n > maxPollQuestion {
		return pollPayload{}, ErrInvalidInput
	}
	if err := allowedText("question", question); err != nil {
		return pollPayload{}, err
	}
	if len(c.Options) < minPollOptions || len(c.Options) > maxPollOptions {
		return pollPayload{}, ErrInvalidInput
	}
	p := pollPayload{Question: question, MultipleChoice: c.MultipleChoice, Anonymous: c.Anonymous}
	for i, option := range c.Options {
		clean := text.Plain(option, false)
		if n := utf8.RuneCountInString(clean); n == 0 || n > maxPollOption {
			return pollPayload{}, ErrInvalidInput
		}
		if err := allowedText("options", clean); err != nil {
			return pollPayload{}, err
		}
		p.Options = append(p.Options, pollOptionPayload{ID: i + 1, Text: clean})
	}
	if c.ClosesAt != nil {
		closesAt := c.ClosesAt.UTC()
		if !closesAt.After(now) || closesAt.After(now.Add(maxPollDuration)) {
			return pollPayload{}, ErrInvalidInput
		}
		p.ClosesAt = &closesAt
	}
	return p, nil
}

type voteChange struct {
	MessageID uuid.UUID
	OptionIDs []int
}

func (p pollPayload) closed(now time.Time) bool {
	return p.ClosedAt != nil || (p.ClosesAt != nil && !p.ClosesAt.After(now))
}

func (p pollPayload) validVote(ids []int) bool {
	if len(ids) > 1 && !p.MultipleChoice {
		return false
	}
	seen := map[int]bool{}
	for _, id := range ids {
		if id < 1 || id > len(p.Options) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func (m Module) votePoll(ctx context.Context, viewerID uuid.UUID, c voteChange) (*Message, error) {
	ids := slices.Clone(c.OptionIDs)
	slices.Sort(ids)

	var notes []notification
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		row, _, err := m.lockCard(ctx, viewerID, c.MessageID, MessagePoll)
		if err != nil {
			return err
		}
		var poll pollPayload
		if err := json.Unmarshal(row.Payload, &poll); err != nil {
			return fmt.Errorf("decode poll: %w", err)
		}
		if !poll.validVote(ids) {
			return ErrInvalidInput
		}
		if poll.closed(m.now()) {
			return ErrPollClosed
		}
		current, err := m.polls.myVote(ctx, c.MessageID, viewerID)
		if err != nil {
			return err
		}
		if slices.Equal(current, ids) {
			return nil
		}
		if err := m.polls.replaceVote(ctx, c.MessageID, viewerID, ids); err != nil {
			return err
		}
		notes, err = m.touchMessage(ctx, c.MessageID)
		return err
	})
	if err != nil {
		return nil, err
	}
	m.publish(ctx, notes)
	return m.getMessage(ctx, viewerID, c.MessageID)
}

func (m Module) closePoll(ctx context.Context, viewerID, messageID uuid.UUID) (*Message, error) {
	var notes []notification
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		row, a, err := m.lockCard(ctx, viewerID, messageID, MessagePoll)
		if err != nil {
			return err
		}
		if (row.SenderID == nil || *row.SenderID != viewerID) && a.Role != RoleOwner {
			return ErrForbidden
		}
		closed, err := m.polls.close(ctx, messageID)
		if err != nil || !closed {
			return err
		}
		notes, err = m.touchMessage(ctx, messageID)
		return err
	})
	if err != nil {
		return nil, err
	}
	m.publish(ctx, notes)
	return m.getMessage(ctx, viewerID, messageID)
}

func (m Module) pollViews(ctx context.Context, viewerID uuid.UUID, rows []messageRow, now time.Time) (map[uuid.UUID]*Poll, error) {
	polls := map[uuid.UUID]*Poll{}
	payloads := map[uuid.UUID]pollPayload{}
	var ids, public []uuid.UUID
	for _, row := range rows {
		if row.Kind != MessagePoll || row.DeletedAt != nil {
			continue
		}
		var p pollPayload
		if err := json.Unmarshal(row.Payload, &p); err != nil {
			return nil, fmt.Errorf("decode poll: %w", err)
		}
		payloads[row.ID] = p
		ids = append(ids, row.ID)
		if !p.Anonymous {
			public = append(public, row.ID)
		}
	}
	if len(ids) == 0 {
		return polls, nil
	}

	counts, err := m.polls.counts(ctx, ids)
	if err != nil {
		return nil, err
	}
	totals, err := m.polls.totals(ctx, ids)
	if err != nil {
		return nil, err
	}
	mine := map[uuid.UUID][]int{}
	if viewerID != uuid.Nil {
		if mine, err = m.polls.votesOf(ctx, ids, viewerID); err != nil {
			return nil, err
		}
	}
	voterIDs := map[uuid.UUID][]uuid.UUID{}
	if viewerID != uuid.Nil && len(public) > 0 {
		if voterIDs, err = m.polls.recentVoters(ctx, public, viewerID, shownVoters); err != nil {
			return nil, err
		}
	}
	var everyone []uuid.UUID
	for _, list := range voterIDs {
		everyone = append(everyone, list...)
	}
	students, err := m.studentMap(ctx, everyone)
	if err != nil {
		return nil, err
	}

	for id, p := range payloads {
		poll := &Poll{
			Question:       p.Question,
			Options:        make([]PollOption, 0, len(p.Options)),
			MultipleChoice: p.MultipleChoice,
			Anonymous:      p.Anonymous,
			ClosesAt:       p.ClosesAt,
			ClosedAt:       p.ClosedAt,
			Closed:         p.closed(now),
			TotalVoters:    totals[id],
			Voters:         make([]Student, 0, len(voterIDs[id])),
			MyVote:         nonNilInts(mine[id]),
		}
		for _, o := range p.Options {
			poll.Options = append(poll.Options, PollOption{ID: o.ID, Text: o.Text, Votes: counts[id][o.ID]})
		}
		for _, voter := range voterIDs[id] {
			poll.Voters = append(poll.Voters, studentOf(students, &voter))
		}
		polls[id] = poll
	}
	return polls, nil
}

func nonNilInts(ids []int) []int {
	if ids == nil {
		return []int{}
	}
	return ids
}
