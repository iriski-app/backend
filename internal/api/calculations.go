package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/service"
	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm"
)

func (h *Handler) GetCalculation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	calcID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest,
			"invalid_calculation_id", "Некорректный ID расчёта", nil)
		return
	}

	cookie, err := r.Cookie("session_id")
	if errors.Is(err, http.ErrNoCookie) {
		writeError(w, http.StatusNotFound,
			"calculation_not_found", "Расчёт не найден", nil)
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "failed to read session cookie", "error", err)
		writeError(w, http.StatusInternalServerError,
			"internal_error", "Внутренняя ошибка сервера", nil)
		return
	}
	sessionID, err := uuid.Parse(cookie.Value)
	if err != nil {
		writeError(w, http.StatusBadRequest,
			"session_error", "Некорректный ID сессии", nil)
		return
	}

	calcRun, calcResults, err := h.Service.GetCalculation(ctx, sessionID, calcID)
	if err != nil {
		if errors.Is(err, service.ErrCalculationNotFound) {
			writeError(w, http.StatusNotFound,
				"calculation_not_found", "Расчёт не найден", nil)
		} else {
			slog.ErrorContext(ctx, "get calculation failed", "error", err)
			writeError(w, http.StatusInternalServerError,
				"internal_error", "Внутренняя ошибка сервера", nil)
		}
		return
	}

	response := newCalculationResponse(calcRun, calcResults)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.ErrorContext(ctx, "failed to encode calculation response", "error", err)
	}
}

func (h *Handler) Calculation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cookie, err := r.Cookie("session_id")
	var sessionId uuid.UUID

	switch {
	case errors.Is(err, http.ErrNoCookie):
		sessionId = createSessionCookie(w)

	case err != nil:
		slog.ErrorContext(ctx, "failed to read session cookie", "error", err)
		writeError(w, http.StatusInternalServerError,
			"internal_error", "Внутренняя ошибка сервера", nil)
		return

	default:
		sessionId, err = uuid.Parse(cookie.Value)
		if err != nil {
			writeError(w, http.StatusBadRequest,
				"session_error", "Некорректный ID сессии", nil)
			return
		}
	}

	var req CalculationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(
			w,
			http.StatusBadRequest,
			"invalid_json",
			"Неверный формат JSON",
			nil,
		)
		return
	}
	positions := make([]service.PositionInput, len(req.Positions))
	for i, position := range req.Positions {
		curInstrument, err := h.Service.GetInstrument(ctx, position.ISIN, models.SourceMoex)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrInvalidISIN):
				writeError(w, http.StatusBadRequest,
					"invalid_isin", "Некорректный ISIN", nil)

			case errors.Is(err, service.ErrInstrumentNotFound):
				writeError(w, http.StatusNotFound,
					"instrument_not_found", "Инструмент не найден", nil)

			default:
				slog.ErrorContext(ctx, "instrument lookup failed", "error", err)
				writeError(w, http.StatusInternalServerError,
					"internal_error", "Внутренняя ошибка сервера", nil)
			}
			return
		}
		positions[i] = service.PositionInput{
			InstrumentID: curInstrument.Instrument.ID,
			Side:         position.Side,
			Quantity:     position.Quantity,
		}
	}
	calcRun, calcResults, err := h.Service.CalculatePositions(
		ctx,
		sessionId,
		positions,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidParameter):
			writeError(
				w,
				http.StatusBadRequest,
				"invalid_parameter",
				"Проверьте количество позиций, количество бумаг и сторону сделки",
				nil,
			)

		case errors.Is(err, service.ErrMissingParameter):
			writeError(
				w,
				http.StatusUnprocessableEntity,
				"missing_parameter",
				"Недостаточно рыночных данных для расчёта",
				nil,
			)

		case errors.Is(err, service.ErrPlaceHolder):
			writeError(
				w,
				http.StatusUnprocessableEntity,
				"missing_duration",
				"Отсутствует дюрация для расчёта",
				nil,
			)

		case errors.Is(err, gorm.ErrRecordNotFound):
			writeError(
				w,
				http.StatusUnprocessableEntity,
				"calculation_data_not_found",
				"Не найден инструмент или рыночные данные для одной из позиций",
				nil,
			)

		default:
			slog.ErrorContext(ctx, "calculation failed", "error", err)
			writeError(
				w,
				http.StatusInternalServerError,
				"internal_error",
				"Внутренняя ошибка сервера",
				nil,
			)
		}
		return
	}
	response := newCalculationResponse(calcRun, calcResults)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.ErrorContext(ctx, "failed to encode calculation response", "error", err)
	}
}

func newCalculationResponse(calcRun models.CalcRun, calcResults []models.CalcResult) CalculationResponse {
	response := CalculationResponse{
		ID:           calcRun.ID,
		Status:       calcRun.Status,
		AsOfDate:     calcRun.AsOfDate,
		TotalDV01Rub: calcRun.TotalDV01Rub,
		Positions:    make([]CalculationPositionResponse, len(calcResults)),
	}
	for i, result := range calcResults {
		response.Positions[i] = CalculationPositionResponse{
			ID:           result.ID,
			InstrumentID: result.InstrumentID,
			Side:         result.Side,
			Quantity:     result.Quantity,
			MarketValue:  result.FairValue,
			DurationMod:  result.DurationMod,
			DV01:         result.DV01,
			Source:       result.DataSource,
			MarketDate:   result.MarketDate,
		}
	}

	return response
}
