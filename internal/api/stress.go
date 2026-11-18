package api

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/service"
)

func (h *Handler) StressScenario(w http.ResponseWriter, r *http.Request) {
	calcID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_calculation_id", "Некорректный ID расчёта", nil)
		return
	}
	cookie, err := r.Cookie("session_id")
	if err != nil {
		writeError(w, http.StatusNotFound, "calculation_not_found", "Расчёт не найден", nil)
		return
	}
	sessionID, err := uuid.Parse(cookie.Value)
	if err != nil {
		writeError(w, http.StatusBadRequest, "session_error", "Некорректный ID сессии", nil)
		return
	}
	var request struct {
		ShiftBP   *float64 `json:"shift_bp"`
		IncludeML bool     `json:"include_ml_analysis"`
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil || request.ShiftBP == nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Укажите сдвиг доходности в bp", nil)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_json", "Некорректный JSON", nil)
		return
	}
	if math.IsNaN(*request.ShiftBP) || math.IsInf(*request.ShiftBP, 0) || *request.ShiftBP < -200 || *request.ShiftBP > 200 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_shift", "Сдвиг должен быть от -200 до 200 bp", nil)
		return
	}
	result, err := h.Service.CalculateStress(r.Context(), sessionID, calcID, *request.ShiftBP)
	if errors.Is(err, service.ErrCalculationNotFound) {
		writeError(w, http.StatusNotFound, "calculation_not_found", "Расчёт не найден", nil)
		return
	}
	if errors.Is(err, service.ErrMissingParameter) || errors.Is(err, service.ErrInvalidParameter) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_calculation", "Недостаточно данных для сценария", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера", nil)
		return
	}
	status := "not_requested"
	if request.IncludeML {
		status = "unavailable"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		service.StressResult
		MLAnalysis map[string]string `json:"ml_analysis"`
	}{result, map[string]string{"status": status}})
}
