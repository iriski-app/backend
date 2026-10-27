package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details"`
}

func writeError(
	w http.ResponseWriter,
	status int,
	code string,
	message string,
	details any,
) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	response := ErrorResponse{
		Code:    code,
		Message: message,
		Details: details,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Error("failed to encode error response", "error", err)
	}
}
