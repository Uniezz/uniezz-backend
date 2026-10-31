package profile

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestReportTarget(t *testing.T) {
	e := newTestEnv(t)
	u := e.otpUser(t)

	snapshot, err := e.module.ReportTarget(context.Background(), u.id)
	if err != nil {
		t.Fatal(err)
	}
	if name, _ := snapshot["firstName"].(*string); name == nil || *name != "Jan" {
		t.Errorf("snapshot = %v", snapshot)
	}

	if _, ok := snapshot["email"]; ok {
		t.Error("the snapshot must not carry the email")
	}

	if _, err := e.module.ReportTarget(context.Background(), uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing user: error = %v, want ErrNotFound", err)
	}
}
