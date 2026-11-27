package chat

import (
	"errors"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrInvalidInput      = errors.New("invalid input")
	ErrContentNotAllowed = errors.New("content not allowed")
	ErrForbidden         = errors.New("only the group owner may do this")
	ErrInvalidMember     = errors.New("members must be your own unblocked connections")
	ErrGroupFull         = errors.New("the group is full")
	ErrInvalidMedia      = errors.New("attachment is missing, not owned by the user or not uploaded")
	ErrKeyReused         = errors.New("idempotency key was used with a different request")
	ErrResourceDeleted   = errors.New("the message created with this idempotency key was deleted")
	ErrRateLimited       = errors.New("too many requests")
	ErrRequestPending    = errors.New("until the recipient accepts, a message request holds one text message without links")
	ErrRequestRequired   = errors.New("a conversation with this student starts with a message request")
	ErrDirectExists      = errors.New("a direct conversation with this student already exists")
	ErrTooManyStreams    = errors.New("too many open streams")
	ErrDraining          = errors.New("the server is shutting down")
	ErrPollClosed        = errors.New("the poll is closed")
	ErrEventCanceled     = errors.New("the event was canceled")
	ErrEventEnded        = errors.New("the event has ended")
	ErrLocationNotLive   = errors.New("the location is not shared live any more")
	ErrSearchTooBroad    = errors.New("the search took too long; add or lengthen words")
)

var errorResponses = httpapi.ErrorResponses{
	{Err: ErrNotFound, Status: http.StatusNotFound, Code: "not_found"},
	{Err: ErrInvalidInput, Status: http.StatusBadRequest, Code: "invalid_input"},
	{Err: ErrContentNotAllowed, Status: http.StatusBadRequest, Code: "content_not_allowed"},
	{Err: ErrForbidden, Status: http.StatusForbidden, Code: "forbidden"},
	{Err: ErrInvalidMember, Status: http.StatusBadRequest, Code: "invalid_member"},
	{Err: ErrGroupFull, Status: http.StatusConflict, Code: "group_full"},
	{Err: ErrInvalidMedia, Status: http.StatusBadRequest, Code: "invalid_media"},
	{Err: ErrKeyReused, Status: http.StatusUnprocessableEntity, Code: "idempotency_key_reused"},
	{Err: ErrResourceDeleted, Status: http.StatusGone, Code: "resource_deleted"},
	{Err: ErrRateLimited, Status: http.StatusTooManyRequests, Code: "rate_limited"},
	{Err: ErrRequestPending, Status: http.StatusConflict, Code: "request_pending"},
	{Err: ErrRequestRequired, Status: http.StatusConflict, Code: "request_required"},
	{Err: ErrDirectExists, Status: http.StatusConflict, Code: "conversation_exists"},
	{Err: ErrTooManyStreams, Status: http.StatusServiceUnavailable, Code: "too_many_streams"},
	{Err: ErrDraining, Status: http.StatusTooManyRequests, Code: "shutting_down"},
	{Err: ErrPollClosed, Status: http.StatusConflict, Code: "poll_closed"},
	{Err: ErrEventCanceled, Status: http.StatusConflict, Code: "event_canceled"},
	{Err: ErrEventEnded, Status: http.StatusConflict, Code: "event_ended"},
	{Err: ErrLocationNotLive, Status: http.StatusConflict, Code: "location_not_live"},
	{Err: ErrSearchTooBroad, Status: http.StatusUnprocessableEntity, Code: "search_too_broad"},
	{Err: httpapi.ErrInvalidCursor, Status: http.StatusBadRequest, Code: "invalid_cursor"},
}
