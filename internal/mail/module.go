package mail

import (
	"errors"
	"os"

	"github.com/resend/resend-go/v2"
)

type Config struct {
	apiKey string
	from string
}

func LoadConfig() (Config, error) {
	apiKey := os.Getenv("key")
	from := os.Getenv("mail")

	if apiKey == "" {
		return Config{}, errors.New("key is required")
	}
	if from == "" {
		return Config{}, errors.New("mail is required")
	}

	return Config{
		apiKey: apiKey,
		from: from,
	}, nil
}

func New(cfg Config) *Sender {
	client := resend.NewClient(cfg.apiKey)
	return &Sender{
		client: client,
		from:   cfg.from,
	}
}