package rewards

import (
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

var errorResponses = httpapi.ErrorResponses{
	{Err: httpapi.ErrInvalidCursor, Status: http.StatusBadRequest, Code: "invalid_cursor"},
}
