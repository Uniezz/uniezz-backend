// Package httpapi holds the Huma setup shared by every module: the API config
// and the {"error":"code"} error format.
package httpapi

import (
	"log"
	"net/http"
	"strings"

	"github.com/Uniezz/uniezz-backend/internal/config"
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
	if cfg.Env == "production" {
		c.DocsPath = ""
	}
	return c
}

type ErrorBody struct {
	status int
	Code   string `json:"error" example:"invalid_input" doc:"Machine-readable error code"`
}

func (e *ErrorBody) Error() string  { return e.Code }
func (e *ErrorBody) GetStatus() int { return e.status }

func Error(status int, code string, err error) huma.StatusError {
	if status >= http.StatusInternalServerError {
		log.Printf("httpapi: %s: %v", code, err)
	}
	return &ErrorBody{status: status, Code: code}
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
		log.Printf("httpapi: %s: %v", msg, errs)
		return &ErrorBody{status: status, Code: "internal_error"}
	default:
		return &ErrorBody{status: status, Code: statusCode(status)}
	}
}

func statusCode(status int) string {
	return strings.ReplaceAll(strings.ToLower(http.StatusText(status)), " ", "_")
}
