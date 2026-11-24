package push

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExpoClient(t *testing.T) {
	t.Run("returns the tokens Expo reports as not registered and sends the access token", func(t *testing.T) {
		var auth string
		var got []expoMessage
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth = r.Header.Get("Authorization")
			_ = json.NewDecoder(r.Body).Decode(&got)
			_, _ = w.Write([]byte(`{"data":[{"status":"ok","id":"1"},{"status":"error","message":"gone","details":{"error":"DeviceNotRegistered"}}]}`))
		}))
		defer server.Close()

		client := &expoClient{http: server.Client(), url: server.URL, accessToken: "secret"}
		invalid, err := client.send(context.Background(), []expoMessage{{To: "ExponentPushToken[a]"}, {To: "ExponentPushToken[b]"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(invalid) != 1 || invalid[0] != "ExponentPushToken[b]" {
			t.Errorf("invalid = %v, want the second token", invalid)
		}
		if auth != "Bearer secret" || len(got) != 2 {
			t.Errorf("auth = %q, messages = %d", auth, len(got))
		}
	})

	t.Run("an error answer from Expo is an error, so the job retries", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer server.Close()

		client := &expoClient{http: server.Client(), url: server.URL}
		if _, err := client.send(context.Background(), []expoMessage{{To: "ExponentPushToken[a]"}}); err == nil {
			t.Errorf("expected an error")
		}
	})
}
