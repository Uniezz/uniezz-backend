package mail

import "github.com/resend/resend-go/v2"

func NewResendSender(apiKey, from string) *Sender {
    return NewSender(resend.NewClient(apiKey).Emails, from)
}