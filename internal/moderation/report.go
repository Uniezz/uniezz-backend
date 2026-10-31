package moderation

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/Uniezz/uniezz-backend/internal/feed"
	"github.com/Uniezz/uniezz-backend/internal/profile"
	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
)

const (
	targetUser       = "user"
	maxDetailsLength = 1000
)

type reportInput struct {
	TargetType string
	TargetID   uuid.UUID
	Reason     string
	Details    *string
}

// createReport validates the target before it counts against the reporter's
// limit, and counts it in the transaction that stores the report, so a
// duplicate or invalid report does not use up the quota.
func (m Module) createReport(ctx context.Context, reporterID uuid.UUID, in reportInput) (*Report, error) {
	details, err := cleanDetails(in.Details)
	if err != nil {
		return nil, err
	}

	authorID, snapshot, err := m.target(ctx, in.TargetType, in.TargetID)
	if err != nil {
		return nil, err
	}
	if authorID == reporterID {
		return nil, ErrSelfReport
	}

	reported, err := m.reports.exists(ctx, reporterID, in.TargetType, in.TargetID)
	if err != nil {
		return nil, err
	}
	if reported {
		return nil, ErrAlreadyReport
	}

	var row *reportRow
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.limiter.Allow(ctx, reporterID.String()); err != nil {
			if errors.Is(err, ratelimit.ErrLimited) {
				return errors.Join(ErrRateLimited, err)
			}
			return err
		}
		var err error
		row, err = m.reports.insert(ctx, newReport{
			ReporterID: reporterID, TargetType: in.TargetType, TargetID: in.TargetID,
			TargetAuthorID: authorID, Reason: in.Reason, Details: details, Snapshot: snapshot,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return &Report{
		ID:         row.ID,
		TargetType: row.TargetType,
		TargetID:   row.TargetID,
		Reason:     row.Reason,
		Details:    row.Details,
		Status:     row.Status,
		CreatedAt:  row.CreatedAt,
	}, nil
}

func (m Module) target(ctx context.Context, targetType string, id uuid.UUID) (uuid.UUID, map[string]any, error) {
	if targetType == targetUser {
		snapshot, err := m.profiles.ReportTarget(ctx, id)
		if errors.Is(err, profile.ErrNotFound) {
			return uuid.Nil, nil, ErrNotFound
		}
		return id, snapshot, err
	}

	authorID, snapshot, err := m.feed.ReportTarget(ctx, targetType, id)
	if errors.Is(err, feed.ErrNotFound) {
		return uuid.Nil, nil, ErrNotFound
	}
	return authorID, snapshot, err
}

func cleanDetails(details *string) (*string, error) {
	if details == nil {
		return nil, nil
	}
	clean := text.Plain(*details, true)
	if utf8.RuneCountInString(clean) > maxDetailsLength {
		return nil, ErrInvalidInput
	}
	if clean == "" {
		return nil, nil
	}
	return &clean, nil
}
