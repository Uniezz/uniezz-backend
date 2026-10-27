// Package httpapi holds the Huma setup shared by every module: the API config
// and the {"error":"code"} error format.
package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/telemetry"
	"github.com/danielgtaylor/huma/v2"
)

const BearerAuth = "bearer"

var Version = "dev"

func NewConfig(cfg *config.Config) huma.Config {
	c := huma.DefaultConfig("Uniezz API", Version)
	if cfg.BackendPublicURL != "" {
		c.Servers = []*huma.Server{{URL: cfg.BackendPublicURL}}
	}
	c.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		BearerAuth: {Type: "http", Scheme: "bearer"},
	}
	c.CreateHooks = nil
	c.Transformers = append(c.Transformers, reportServerErrors)
	if cfg.Env == "production" {
		c.DocsPath = ""
	}
	return c
}

type ErrorBody struct {
	status int
	cause  error
	Code   string `json:"error" example:"invalid_input" doc:"Machine-readable error code"`
}

func (e *ErrorBody) Error() string  { return e.Code }
func (e *ErrorBody) GetStatus() int { return e.status }

func Error(status int, code string, err error) huma.StatusError {
	return &ErrorBody{status: status, cause: err, Code: code}
}

func reportServerErrors(ctx huma.Context, _ string, v any) (any, error) {
	e, ok := v.(*ErrorBody)
	if !ok || e.status < http.StatusInternalServerError {
		return v, nil
	}

	cause := e.cause
	if cause == nil {
		cause = errors.New(e.Code)
	}
	telemetry.ReportError(ctx.Context(), "server error", cause, slog.String("error_code", e.Code))
	return v, nil
}

func init() {
	huma.NewError = newError
}

func newError(status int, msg string, errs ...error) huma.StatusError {
	switch {
	case status == 0:
		return &ErrorBody{}
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		return &ErrorBody{status: http.StatusBadRequest, Code: "invalid_input"}
	case status >= http.StatusInternalServerError:
		cause := errors.Join(errs...)
		if cause == nil {
			cause = errors.New(msg)
		} else {
			cause = errors.Join(errors.New(msg), cause)
		}
		return &ErrorBody{status: status, cause: cause, Code: "internal_error"}
	default:
		return &ErrorBody{status: status, Code: statusCode(status)}
	}
}

func statusCode(status int) string {
	return strings.ReplaceAll(strings.ToLower(http.StatusText(status)), " ", "_")
}
