// Package telegram posts plain-text messages to topics of the team chat. A
// thread without a bot token, chat ID or topic ID sends nothing, so staging and
// local runs stay quiet.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	APIURL       = "https://api.telegram.org"
	sendTimeout  = 5 * time.Second
	maxErrorBody = 4 << 10
)

type Client struct {
	baseURL string
	token   string
	chatID  string
	http    *http.Client
}

func New(baseURL, token, chatID string) Client {
	return Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		chatID:  chatID,
		http:    &http.Client{Timeout: sendTimeout},
	}
}

type Thread struct {
	client Client
	id     int
}

func (c Client) Thread(id int) Thread {
	return Thread{client: c, id: id}
}

func (t Thread) Enabled() bool {
	return t.client.token != "" && t.client.chatID != "" && t.id > 0
}

type sendMessageRequest struct {
	ChatID             string             `json:"chat_id"`
	MessageThreadID    int                `json:"message_thread_id"`
	Text               string             `json:"text"`
	LinkPreviewOptions linkPreviewOptions `json:"link_preview_options"`
}

type linkPreviewOptions struct {
	IsDisabled bool `json:"is_disabled"`
}

type errorAnswer struct {
	Description string `json:"description"`
}

func (t Thread) Send(ctx context.Context, text string) error {
	if !t.Enabled() {
		return nil
	}

	body, err := json.Marshal(sendMessageRequest{
		ChatID:             t.client.chatID,
		MessageThreadID:    t.id,
		Text:               text,
		LinkPreviewOptions: linkPreviewOptions{IsDisabled: true},
	})
	if err != nil {
		return fmt.Errorf("encode message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.client.baseURL+"/bot"+t.client.token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return errors.New("build telegram request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.http.Do(req)
	if err != nil {
		// The *url.Error repeats the request URL, which holds the bot token.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var answer errorAnswer
		_ = json.NewDecoder(io.LimitReader(resp.Body, maxErrorBody)).Decode(&answer)
		if answer.Description != "" {
			return fmt.Errorf("send message: telegram answered %d: %s", resp.StatusCode, answer.Description)
		}
		return fmt.Errorf("send message: telegram answered %d", resp.StatusCode)
	}
	return nil
}
