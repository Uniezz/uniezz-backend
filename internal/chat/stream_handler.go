package chat

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
)

type streamHandler struct {
	chat Module
}

const streamExample = `event: ready
data: {}

event: message.created
data: {"conversationId":"…","changeSeq":12,"message":{…}}

: ping

`

var openChatStreamOperation = auth.Authenticated(huma.Operation{
	OperationID: "openChatStream",
	Method:      http.MethodGet,
	Path:        "/chat/stream",
	Summary:     "Open the live event stream of the logged-in student",
	Description: "Server-Sent Events for every conversation of the student, without subscribing: ready, message.created, message.updated, conversation.updated, conversation.removed, read.updated, reconnect and auth_expired, plus a ping comment every 20 seconds. The stream is not a delivery guarantee: after every (re)connect, catch up through listChatConversations and listChatMessages with changedAfter. A student has at most 5 streams; opening another closes the oldest.",
	Tags:        []string{"chat"},
	Responses: map[string]*huma.Response{
		"200": {
			Description: "An endless text/event-stream",
			Content: map[string]*huma.MediaType{
				"text/event-stream": {Schema: &huma.Schema{Type: "string"}, Example: streamExample},
			},
		},
	},
	Errors: []int{http.StatusTooManyRequests, http.StatusServiceUnavailable},
})

func (h *streamHandler) openStream(ctx context.Context, _ *struct{}) (*huma.StreamResponse, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	sessionID, ok := auth.SessionIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	sub, err := h.chat.hub.subscribe(userID, sessionID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &huma.StreamResponse{Body: func(hctx huma.Context) {
		h.chat.serveStream(hctx, sub)
	}}, nil
}

func (m Module) serveStream(hctx huma.Context, sub *subscriber) {
	defer m.hub.unsubscribe(sub)

	hctx.SetHeader("Content-Type", "text/event-stream")
	hctx.SetHeader("Cache-Control", "no-cache")
	hctx.SetHeader("X-Accel-Buffering", "no")

	_, w := humachi.Unwrap(hctx)
	rc := http.NewResponseController(w)
	go func() {
		<-sub.done
		_ = rc.SetWriteDeadline(time.Now().Add(m.closeWait))
	}()
	write := func(frame []byte) bool {
		deadline := writeDeadline
		select {
		case <-sub.done:
			deadline = m.closeWait
		default:
		}
		_ = rc.SetWriteDeadline(time.Now().Add(deadline))
		if _, err := w.Write(frame); err != nil {
			return false
		}
		return rc.Flush() == nil
	}

	if !write(readyFrame) {
		return
	}

	heartbeat := time.NewTicker(m.heartbeat)
	defer heartbeat.Stop()
	sessionCheck := time.NewTicker(m.sessionCheck)
	defer sessionCheck.Stop()

	ctx := hctx.Context()
	for {
		select {
		case frame := <-sub.events:
			if !write(frame) {
				return
			}
		case <-heartbeat.C:
			if !write(heartbeatFrame) {
				return
			}
		case <-sessionCheck.C:
			active, err := m.sessions.active(ctx, sub.sessionID)
			if err != nil {
				slog.WarnContext(ctx, "check stream session", slog.Any("error", err))
				continue
			}
			if !active {
				write(authExpiredFrame)
				return
			}
		case <-sub.done:
			for {
				select {
				case frame := <-sub.events:
					if !write(frame) {
						return
					}
				default:
					return
				}
			}
		case <-ctx.Done():
			return
		}
	}
}
