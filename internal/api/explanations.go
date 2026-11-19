package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (h *Handler) ExplainCalculation(w http.ResponseWriter, r *http.Request) {
	calcID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "invalid_calculation_id", "Некорректный ID расчёта", nil)
		return
	}
	cookie, err := r.Cookie("session_id")
	if err != nil {
		writeError(w, 404, "calculation_not_found", "Расчёт не найден", nil)
		return
	}
	sessionID, err := uuid.Parse(cookie.Value)
	if err != nil {
		writeError(w, 400, "session_error", "Некорректный ID сессии", nil)
		return
	}
	result, err := h.Service.ExplainDV01(r.Context(), sessionID, calcID)
	switch {
	case errors.Is(err, service.ErrCalculationNotFound):
		writeError(w, 404, "calculation_not_found", "Расчёт не найден", nil)
		return
	case errors.Is(err, service.ErrMissingParameter):
		writeError(w, 422, "invalid_calculation", "Недостаточно данных для объяснения", nil)
		return
	case errors.Is(err, context.DeadlineExceeded) || status.Code(err) == codes.DeadlineExceeded:
		writeError(w, 504, "explanation_timeout", "Время ожидания объяснения истекло", nil)
		return
	case status.Code(err) == codes.DataLoss:
		writeError(w, http.StatusBadGateway, "explanation_unverified", "Объяснение не прошло проверку", nil)
		return
	case errors.Is(err, service.ErrMLUnavailable):
		writeError(w, 503, "explanation_unavailable", "Не удалось получить объяснение", nil)
		return
	case err != nil:
		writeError(w, 500, "internal_error", "Внутренняя ошибка сервера", nil)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(struct {
		ID                          uuid.UUID       `json:"id"`
		CalculationID               uuid.UUID       `json:"calculation_id"`
		Text                        string          `json:"text"`
		Verified                    bool            `json:"validation_ok"`
		FactIDs                     json.RawMessage `json:"sources"`
		ModelVersion                string          `json:"model_version"`
		CreatedAt                   string          `json:"created_at"`
		CalculationDetailsAvailable bool            `json:"calculation_details_available"`
		Limitations                 json.RawMessage `json:"limitations,omitempty"`
	}{
		ID: result.ID, CalculationID: calcID, Text: result.Answer, Verified: result.ValidationOK,
		FactIDs: json.RawMessage(result.FactIDs), ModelVersion: result.GeneratedBy,
		CreatedAt:                   result.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		CalculationDetailsAvailable: result.CalculationDetailsAvailable, Limitations: json.RawMessage(result.Limitations),
	})
}
