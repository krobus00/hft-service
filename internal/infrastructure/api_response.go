package infrastructure

import (
	"encoding/json"
	"net/http"
	"time"
)

type APIResponse struct {
	Message   string    `json:"message,omitempty"`
	Data      any       `json:"data,omitempty"`
	Error     *APIError `json:"error,omitempty"`
	RequestID string    `json:"request_id,omitempty"`
	Timestamp int64     `json:"timestamp"`
}

type APIError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

func WriteSuccess(w http.ResponseWriter, status int, data any, message string) {
	writeAPIResponse(w, status, APIResponse{
		Message:   message,
		Data:      data,
		RequestID: w.Header().Get("X-Request-Id"),
		Timestamp: time.Now().UTC().UnixMilli(),
	})
}

func WriteError(w http.ResponseWriter, status int, code string, message string) {
	writeAPIResponse(w, status, APIResponse{
		Error: &APIError{
			Code:    code,
			Message: message,
		},
		RequestID: w.Header().Get("X-Request-Id"),
		Timestamp: time.Now().UTC().UnixMilli(),
	})
}

func writeAPIResponse(w http.ResponseWriter, status int, payload APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
