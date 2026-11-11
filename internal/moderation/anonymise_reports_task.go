package moderation

import (
	"context"
	"time"
)

const forgottenReportRetention = 60 * 24 * time.Hour

type AnonymiseReportsArgs struct{}

func (AnonymiseReportsArgs) Kind() string { return "moderation.anonymise-reports.v1" }

func (m Module) anonymiseReports(ctx context.Context, _ AnonymiseReportsArgs) error {
	if err := m.reports.markTargetsForgotten(ctx); err != nil {
		return err
	}
	return m.reports.anonymiseForgotten(ctx, forgottenReportRetention)
}
