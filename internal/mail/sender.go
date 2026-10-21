package mail

import (
	"fmt"
	"bytes"
	"context"
	"embed"
	"html/template"

	"github.com/resend/resend-go/v2"
)

var templatesFS embed.FS

var otpTmpl = template.Must(template.ParseFS(templatesFS, "templates/otp.html"))

type emailClient interface {
    SendWithContext(ctx context.Context, p *resend.SendEmailRequest) (*resend.SendEmailResponse, error)
}

type Sender struct {
    client emailClient
    from   string
}

func NewSender(client emailClient, from string) *Sender {
    return &Sender{client: client, from: from}
}

func (s *Sender) SendOTP(to string, code string) error {
	var buf bytes.Buffer

	if err := otpTmpl.Execute(&buf, struct{ Code string }{code}); err != nil {
        return fmt.Errorf("mail: render OTP: %w", err)
    }

    _, err := s.client.SendWithContext(ctx, &resend.SendEmailRequest){
        From:    s.from,
        To:      []string{to},
        Subject: "Verification code",
        Html:    buf.String(),
    })

	if err != nil {
		return fmt.Errorf("mail: send OTP: %w", err)
	}

	return nil
}
