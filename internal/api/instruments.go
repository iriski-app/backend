package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/iriski-app/backend/internal/service"
	"github.com/iriski-app/backend/internal/storage/models"
)

func (h *Handler) GetInstrument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	isin := r.PathValue("isin")
	source := models.Source(r.URL.Query().Get("source"))

	info, err := h.Service.GetInstrument(ctx, isin, source)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidISIN):
			writeError(
				w,
				http.StatusBadRequest,
				"invalid_isin",
				"Некорректный ISIN",
				nil,
			)
		case errors.Is(err, service.ErrInvalidSource):
			writeError(w, http.StatusBadRequest, "invalid_source", "Неподдерживаемый источник данных", nil)
		case errors.Is(err, service.ErrInstrumentNotFound):
			writeError(w, http.StatusNotFound, "instrument_not_found", "Инструмент не найден", nil)
		default:
			slog.ErrorContext(ctx, "get instrument failed", "error", err)
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

	response := GetInstrumentResponse{
		ID:                  info.Instrument.ID,
		ISIN:                info.Instrument.ISIN,
		Name:                info.Instrument.Name,
		InstrumentType:      info.Instrument.InstrumentType,
		IssuerName:          info.Instrument.IssuerName,
		Currency:            info.Instrument.Currency,
		FaceValue:           info.Instrument.FaceValue,
		ReadyForCalculation: info.ReadyForCalculation,
		MissingParameters:   info.MissingParameters,
	}

	if info.MarketData != nil {
		response.PriceClean = info.MarketData.PriceClean
		response.AccruedInt = info.MarketData.AccruedInt
		response.DurationMod = info.MarketData.DurationMod
		response.Source = string(info.MarketData.Source)
		response.MarketDate = &info.MarketData.TradeDate
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.ErrorContext(ctx, "failed to encode instrument response", "error", err)
	}
}
