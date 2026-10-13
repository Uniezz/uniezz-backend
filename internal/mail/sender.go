package mail

import (
	"fmt"

	"github.com/resend/resend-go/v2"
)

type Sender struct {
	client *resend.Client
	from   string
}

func (s *Sender) SendOTP(to string, code string) error {
	params := &resend.SendEmailRequest{
		From:    s.from,
		To:      []string{to},
		Subject: "Verification code",
		Html: fmt.Sprintf(`
			<p>Your verification code is: <strong>%s</strong></p>
		`, code),
	}

	_, err := s.client.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("failed to deliver OTP to %s: %w", to, err)
	}

	return nil
}
