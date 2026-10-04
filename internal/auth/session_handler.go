package auth

import (
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpjson"
)

type sessionHandler struct {
	logins *logins
}

type exchangeRequest struct {
	Code string `json:"code"`
}

func (h *sessionHandler) exchange(w http.ResponseWriter, r *http.Request) {
	var req exchangeRequest
	if err := httpjson.Decode(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}

	session, err := h.logins.exchange(r.Context(), req.Code)
	if err != nil {
		writeError(w, r, err)
		return
	}

	httpjson.Write(w, http.StatusOK, session)
}
