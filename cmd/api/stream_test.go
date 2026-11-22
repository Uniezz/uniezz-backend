package main

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
)

const (
	testWriteTimeout = 300 * time.Millisecond
	testStreamLength = 1500 * time.Millisecond
	testHeartbeat    = 50 * time.Millisecond
)

func streamThroughRouter(t *testing.T, extendDeadline bool) (pings int, elapsed time.Duration) {
	t.Helper()

	r, api := newRouter(&config.Config{})
	huma.Register(api, huma.Operation{Method: http.MethodGet, Path: "/test-stream"},
		func(_ context.Context, _ *struct{}) (*huma.StreamResponse, error) {
			return &huma.StreamResponse{Body: func(ctx huma.Context) {
				ctx.SetHeader("Content-Type", "text/event-stream")
				_, w := humachi.Unwrap(ctx)
				rc := http.NewResponseController(w)

				end := time.Now().Add(testStreamLength)
				for time.Now().Before(end) {
					if extendDeadline {
						if err := rc.SetWriteDeadline(time.Now().Add(testWriteTimeout)); err != nil {
							t.Errorf("SetWriteDeadline did not reach the connection through the middleware: %v", err)
							return
						}
					}
					if _, err := w.Write([]byte(": ping\n\n")); err != nil {
						return
					}
					if err := rc.Flush(); err != nil {
						t.Errorf("Flush did not reach the connection through the middleware: %v", err)
						return
					}
					time.Sleep(testHeartbeat)
				}
			}}, nil
		})

	srv := httptest.NewUnstartedServer(r)
	srv.Config.WriteTimeout = testWriteTimeout
	srv.Start()
	defer srv.Close()

	start := time.Now()
	resp, err := http.Get(srv.URL + "/test-stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), ": ping") {
			pings++
		}
	}
	return pings, time.Since(start)
}

func TestStreamThroughRouterOutlivesTheServerWriteTimeout(t *testing.T) {
	pings, elapsed := streamThroughRouter(t, true)

	if elapsed < testStreamLength-testHeartbeat || pings < int(testStreamLength/testHeartbeat)-3 {
		t.Errorf("stream ran %v with %d pings, want it to outlive the %v write timeout through every middleware", elapsed, pings, testWriteTimeout)
	}
}

func TestStreamWithoutAWriteDeadlineIsCutByTheServerWriteTimeout(t *testing.T) {
	_, elapsed := streamThroughRouter(t, false)

	if elapsed > 2*testWriteTimeout {
		t.Errorf("stream ran %v, the server write timeout %v should have cut it", elapsed, testWriteTimeout)
	}
}
