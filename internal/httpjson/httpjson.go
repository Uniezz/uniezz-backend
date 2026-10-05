package httpjson

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
)

const maxBodyBytes = 1 << 20

var ErrInvalidBody = errors.New("invalid request body")

type errorBody struct {
	Error string `json:"error"`
}

func Write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("httpjson: failed to write response: %v", err)
	}
}

func Error(w http.ResponseWriter, r *http.Request, status int, code string, err error) {
	if status >= http.StatusInternalServerError {
		log.Printf("%s %s: %v", r.Method, r.URL.Path, err)
	}
	Write(w, status, errorBody{Error: code})
}

func Decode(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidBody, err)
	}
	return nil
}
