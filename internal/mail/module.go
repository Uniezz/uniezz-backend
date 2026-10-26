package mail

import (
	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/resend/resend-go/v4"
)

func NewResendSender(cfg *config.Config) *Sender {
	return NewSender(resend.NewClient(cfg.ResendAPIKey).Emails, cfg.MailFrom, cfg.OTPEmailTemplateID)
}
