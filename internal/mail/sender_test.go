package mail

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/resend/resend-go/v4"
)

type fakeClient struct {
	calls int
	ctx   context.Context
	got   *resend.SendEmailRequest
	err   error
}

func (f *fakeClient) SendWithContext(ctx context.Context, p *resend.SendEmailRequest) (*resend.SendEmailResponse, error) {
	f.calls++
	f.ctx, f.got = ctx, p
	return &resend.SendEmailResponse{}, f.err
}

func TestSendOTP_BuildsRequest(t *testing.T) {
	fc := &fakeClient{}
	s := NewSender(fc, "from@gmail.com", "tmpl_otp")

	if err := s.SendOTP(context.Background(), "u@gmail.com", "123456"); err != nil {
		t.Fatal(err)
	}
	if fc.calls != 1 {
		t.Fatalf("want 1 attempt, got %d", fc.calls)
	}
	if fc.got.From != "from@gmail.com" || len(fc.got.To) != 1 || fc.got.To[0] != "u@gmail.com" {
		t.Fatalf("bad addresses: %+v", fc.got)
	}
	if fc.got.Template == nil || fc.got.Template.Id != "tmpl_otp" {
		t.Fatalf("expected template tmpl_otp, got %+v", fc.got.Template)
	}
	if got := fc.got.Template.Variables["code"]; got != "123456" {
		t.Fatalf("expected code variable 123456, got %v", got)
	}
	if fc.got.Html != "" || fc.got.Text != "" {
		t.Fatal("html/text must be empty when sending a hosted template")
	}
}

type ctxKey struct{}

func TestSendOTP_ForwardsContext(t *testing.T) {
	fc := &fakeClient{}
	s := NewSender(fc, "from@gmail.com", "tmpl_otp")
	ctx := context.WithValue(context.Background(), ctxKey{}, "marker")

	_ = s.SendOTP(ctx, "u@gmail.com", "123456")

	if fc.ctx != ctx {
		t.Fatal("context was not forwarded as is")
	}
}

func TestSendOTP_ContextErrorsStayInspectable(t *testing.T) {
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		s := NewSender(&fakeClient{err: want}, "from@gmail.com", "tmpl_otp")
		err := s.SendOTP(context.Background(), "u@gmail.com", "123456")
		if !errors.Is(err, want) {
			t.Fatalf("want errors.Is(%v), got %v", want, err)
		}
	}
}

func TestSendOTP_WrapsProviderError(t *testing.T) {
	boom := errors.New("boom")
	fc := &fakeClient{err: boom}
	s := NewSender(fc, "from@gmail.com", "tmpl_otp")

	err := s.SendOTP(context.Background(), "u@gmail.com", "123456")

	if !errors.Is(err, boom) {
		t.Fatalf("error not wrapped: %v", err)
	}
	if strings.Contains(err.Error(), "u@gmail.com") {
		t.Fatal("recipient leaked into error")
	}
	if fc.calls != 1 {
		t.Fatalf("no retries expected, got %d calls", fc.calls)
	}
}
