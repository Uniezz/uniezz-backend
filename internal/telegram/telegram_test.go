package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testToken  = "123456:bot-secret-token"
	testChatID = "-1001234567890"
)

func TestThreadSend(t *testing.T) {
	t.Run("posts the text to the chat topic with link previews off", func(t *testing.T) {
		var got sendMessageRequest
		var raw map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/bot"+testToken+"/sendMessage" {
				t.Errorf("request = %s %s, want POST /bot<token>/sendMessage", r.Method, r.URL.Path)
			}
			if ct := r.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &got); err != nil {
				t.Errorf("decode body: %v", err)
			}
			_ = json.Unmarshal(body, &raw)
		}))
		t.Cleanup(server.Close)

		if err := New(server.URL, testToken, testChatID).Thread(825).Send(context.Background(), "hello"); err != nil {
			t.Fatalf("send: %v", err)
		}
		if got.ChatID != testChatID || got.MessageThreadID != 825 || got.Text != "hello" || !got.LinkPreviewOptions.IsDisabled {
			t.Errorf("message = %+v", got)
		}
		if _, ok := raw["message_thread_id"]; !ok {
			t.Error("message_thread_id is missing")
		}
	})

	t.Run("without a token, chat id or topic nothing is sent", func(t *testing.T) {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
		t.Cleanup(server.Close)

		for _, th := range []Thread{
			New(server.URL, "", testChatID).Thread(1),
			New(server.URL, testToken, "").Thread(1),
			New(server.URL, testToken, testChatID).Thread(0),
			{},
		} {
			if th.Enabled() {
				t.Errorf("thread %+v is enabled", th)
			}
			if err := th.Send(context.Background(), "x"); err != nil {
				t.Errorf("send: %v", err)
			}
		}
		if calls != 0 {
			t.Errorf("got %d requests, want none", calls)
		}
	})

	t.Run("a non-200 answer is an error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		t.Cleanup(server.Close)

		if err := New(server.URL, testToken, testChatID).Thread(1).Send(context.Background(), "x"); err == nil {
			t.Error("send returned nil for a 429 answer")
		}
	})

	t.Run("a failed post does not leak the bot token", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		url := server.URL
		server.Close()

		err := New(url, testToken, testChatID).Thread(1).Send(context.Background(), "x")
		if err == nil {
			t.Fatal("send to a closed server returned nil")
		}
		if strings.Contains(err.Error(), testToken) {
			t.Errorf("error leaks the bot token: %v", err)
		}
	})
}
