package mail

import (
	"context"
	"fmt"

	"github.com/resend/resend-go/v4"
)

// otpCodeVar is the placeholder name in the Resend-hosted OTP template ({{code}}).
// Resend matches template variables case-sensitively.
const otpCodeVar = "code"

type emailClient interface {
	SendWithContext(ctx context.Context, p *resend.SendEmailRequest) (*resend.SendEmailResponse, error)
}

type Sender struct {
	client        emailClient
	from          string
	otpTemplateID string
}

func NewSender(client emailClient, from, otpTemplateID string) *Sender {
	return &Sender{client: client, from: from, otpTemplateID: otpTemplateID}
}

func (s *Sender) SendOTP(ctx context.Context, to, code string) error {
	_, err := s.client.SendWithContext(ctx, &resend.SendEmailRequest{
		From:    s.from,
		To:      []string{to},
		Subject: "Verification code",
		Template: &resend.EmailTemplate{
			Id:        s.otpTemplateID,
			Variables: map[string]any{otpCodeVar: code},
		},
	})

	if err != nil {
		return fmt.Errorf("mail: send OTP: %w", err)
	}

	return nil
}
