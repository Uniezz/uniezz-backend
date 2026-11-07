package moderation

import (
	"context"
	"time"
)

const forgottenReportRetention = 60 * 24 * time.Hour

type AnonymiseReportsArgs struct{}

func (AnonymiseReportsArgs) Kind() string { return "moderation.anonymise-reports.v1" }

func (m Module) anonymiseReports(ctx context.Context, _ AnonymiseReportsArgs) error {
	return m.reports.anonymiseForgottenOlderThan(ctx, forgottenReportRetention)
}
