package connections

import (
	"context"
	"errors"
	"time"

	"unicode/utf8"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/profile"
	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
)

const (
	modeFriends = "friends"
	modeStudy   = "studyBuddy"
	modeDating  = "dating"
)

type Card struct {
	CandidateID     uuid.UUID `json:"candidateId" format:"uuid"`
	Intent          *string   `json:"intent" nullable:"true"`
	Bio             *string   `json:"bio" nullable:"true"`
	Interests       []string  `json:"interests"`
	SharedInterests []string  `json:"sharedInterests"`
	UniversityID    string    `json:"universityId"`
	Level           *string   `json:"level" nullable:"true"`
	Age             *int      `json:"age" nullable:"true"`
}

type Connection struct {
	ID          uuid.UUID              `json:"id" format:"uuid"`
	Mode        string                 `json:"mode" enum:"friends,studyBuddy,dating"`
	ConnectedAt time.Time              `json:"connectedAt"`
	Profile     *profile.PublicProfile `json:"profile"`
}

type pageCursor struct {
	CreatedAt time.Time `json:"c"`
	ID        uuid.UUID `json:"i"`
}

func (m Module) savePreferences(ctx context.Context, userID uuid.UUID, birth *time.Time, openTo []string, intent *string, setIntent bool) error {
	if birth != nil && birth.After(time.Now()) {
		return ErrInvalidInput
	}
	for _, mode := range openTo {
		if !validMode(mode) {
			return ErrInvalidInput
		}
	}
	if setIntent && intent != nil {
		clean := text.Plain(*intent, false)
		if utf8.RuneCountInString(clean) > 200 {
			return ErrInvalidInput
		}
		if clean == "" {
			intent = nil
		} else {
			intent = &clean
		}
	}
	return m.repo.savePreferences(ctx, userID, preferences{BirthDate: birth, OpenTo: openTo, Intent: intent, SetIntent: setIntent})
}

func (m Module) discover(ctx context.Context, q discoverQuery) ([]Card, error) {
	if !validMode(q.Mode) {
		return nil, ErrInvalidInput
	}
	if q.Limit <= 0 || q.Limit > 20 {
		q.Limit = 20
	}
	rows, err := m.repo.discover(ctx, q)
	if err != nil {
		return nil, err
	}
	mine, err := m.repo.interests(ctx, q.Viewer)
	if err != nil {
		return nil, err
	}
	cards := make([]Card, 0, len(rows))
	for _, row := range rows {
		id, err := m.repo.candidate(ctx, q.Viewer, row.UserID)
		if err != nil {
			return nil, err
		}
		cards = append(cards, Card{
			CandidateID: id, Intent: row.Intent, Bio: row.Bio,
			Interests: nonNil(row.Interests), SharedInterests: overlap(mine, row.Interests),
			UniversityID: row.University, Level: row.Level, Age: row.Age,
		})
	}
	return cards, nil
}

type actionResult struct {
	Matched    bool        `json:"matched"`
	Connection *Connection `json:"connection,omitempty"`
}

func (m Module) act(ctx context.Context, viewer, candidate uuid.UUID, mode, action string) (*actionResult, error) {
	if !validMode(mode) || (action != "like" && action != "pass") {
		return nil, ErrInvalidInput
	}
	target, err := m.repo.lookup(ctx, viewer, candidate)
	if err != nil {
		return nil, err
	}
	if action == "like" {
		if err := m.likes.Allow(ctx, viewer.String()); err != nil {
			if errors.Is(err, ratelimit.ErrLimited) {
				return nil, ErrRateLimited
			}
			return nil, err
		}
	}
	if err := m.repo.act(ctx, viewer, target, mode, action); err != nil {
		return nil, err
	}
	out := &actionResult{}
	if action != "like" {
		return out, nil
	}
	reciprocal, err := m.repo.liked(ctx, target, viewer, mode)
	if err != nil || !reciprocal {
		return out, err
	}
	var created bool
	var row *connectionRow
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		row, created, err = m.repo.match(ctx, viewer, target, mode)
		return err
	})
	if err != nil || row == nil {
		return out, err
	}
	out.Matched = created || row != nil
	conn, err := m.connection(ctx, viewer, *row)
	if err != nil {
		return nil, err
	}
	out.Connection = conn
	out.Matched = true
	return out, nil
}

func (m Module) list(ctx context.Context, userID uuid.UUID, cursor string, limit int) ([]Connection, *string, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var after *pageCursor
	if cursor != "" {
		after = &pageCursor{}
		if err := httpapi.DecodeCursor(cursor, after); err != nil {
			return nil, nil, err
		}
	}
	rows, err := m.repo.list(ctx, userID, after, limit+1)
	if err != nil {
		return nil, nil, err
	}
	rows, next, err := httpapi.Page(rows, limit, func(last connectionRow) any {
		return pageCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	})
	if err != nil {
		return nil, nil, err
	}
	out := make([]Connection, 0, len(rows))
	for _, row := range rows {
		conn, err := m.connection(ctx, userID, row)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, *conn)
	}
	return out, next, nil
}

func (m Module) connection(ctx context.Context, viewer uuid.UUID, row connectionRow) (*Connection, error) {
	shown, err := m.profiles.ShownProfile(ctx, viewer, row.OtherID)
	if err != nil {
		return nil, err
	}
	return &Connection{ID: row.ID, Mode: row.Mode, ConnectedAt: row.CreatedAt, Profile: shown}, nil
}

func (m Module) CandidateReport(ctx context.Context, viewer, candidate uuid.UUID) (uuid.UUID, map[string]any, error) {
	userID, err := m.repo.lookup(ctx, viewer, candidate)
	if err != nil {
		return uuid.Nil, nil, err
	}
	card, err := m.repo.card(ctx, userID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	if err := m.repo.act(ctx, viewer, userID, modeFriends, "hide"); err != nil {
		return uuid.Nil, nil, err
	}
	return userID, map[string]any{"intent": card.Intent, "bio": card.Bio, "interests": nonNil(card.Interests)}, nil
}

func validMode(mode string) bool {
	return mode == modeFriends || mode == modeStudy || mode == modeDating
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func overlap(a, b []string) []string {
	set := map[string]bool{}
	for _, item := range a {
		set[item] = true
	}
	var shared []string
	for _, item := range b {
		if set[item] {
			shared = append(shared, item)
		}
	}
	if shared == nil {
		return []string{}
	}
	return shared
}
