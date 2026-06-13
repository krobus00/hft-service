package api

import (
	"encoding/json"
	"net/http"

	"github.com/krobus00/hft-service/internal/constant"
)

type ValidationError struct {
	Field   string `json:"field"`
	Tag     string `json:"tag"`
	Message string `json:"message"`
}

type BaseResponse struct {
	StatusCode       int               `json:"-"`
	Message          string            `json:"message"`
	Code             string            `json:"code"`
	Data             any               `json:"data,omitempty"`
	ValidationErrors []ValidationError `json:"validation_errors,omitempty"`
}

func (r *BaseResponse) Error() string {
	return r.Message
}

func NewSuccessResponse(data any) *BaseResponse {
	return &BaseResponse{
		StatusCode: http.StatusOK,
		Message:    constant.MessageSuccess,
		Code:       constant.SuccessStatusCode,
		Data:       data,
	}
}

func NewErrorResponse(statusCode int, code string, message string) *BaseResponse {
	return &BaseResponse{
		StatusCode: statusCode,
		Message:    message,
		Code:       code,
	}
}

func WriteJSON(w http.ResponseWriter, resp *BaseResponse) {
	statusCode := resp.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
}

func WriteSuccess(w http.ResponseWriter, data any) {
	WriteJSON(w, NewSuccessResponse(data))
}

func WriteError(w http.ResponseWriter, statusCode int, code string, message string) {
	WriteJSON(w, NewErrorResponse(statusCode, code, message))
}
