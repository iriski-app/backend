package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/service"
	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm"
)

type instrumentTestStore struct {
	service.Storage
	instrument    models.Instrument
	marketData    models.MarketData
	instrumentErr error
	marketErr     error
	source        models.Source
	lookupCalls   int
	marketCalls   int
}

func (s *instrumentTestStore) GetInstrument(_ context.Context, _ string) (models.Instrument, error) {
	s.lookupCalls++
	return s.instrument, s.instrumentErr
}

func (s *instrumentTestStore) GetMarketData(_ context.Context, _ uuid.UUID, source models.Source, _ time.Time) (models.MarketData, error) {
	s.marketCalls++
	s.source = source
	data := s.marketData
	data.Source = source
	return data, s.marketErr
}

func TestGetInstrument(t *testing.T) {
	storageErr := errors.New("database unavailable")
	tests := []struct {
		name          string
		isin          string
		query         string
		instrumentErr error
		marketErr     error
		wantStatus    int
		wantCode      string
		wantSource    models.Source
	}{
		{name: "default source", wantStatus: http.StatusOK, wantSource: models.SourceMoex},
		{name: "selected source", query: "?source=rudata", wantStatus: http.StatusOK, wantSource: models.SourceRuData},
		{name: "no market data", marketErr: gorm.ErrRecordNotFound, wantStatus: http.StatusOK, wantSource: models.SourceMoex},
		{name: "invalid ISIN", isin: "invalid", wantStatus: http.StatusBadRequest, wantCode: "invalid_isin"},
		{name: "invalid check digit", isin: "RU000A0JX0J3", wantStatus: http.StatusBadRequest, wantCode: "invalid_isin"},
		{name: "invalid source", query: "?source=unknown", wantStatus: http.StatusBadRequest, wantCode: "invalid_source"},
		{name: "instrument not found", instrumentErr: gorm.ErrRecordNotFound, wantStatus: http.StatusNotFound, wantCode: "instrument_not_found"},
		{name: "wrapped not found", instrumentErr: fmt.Errorf("lookup: %w", gorm.ErrRecordNotFound), wantStatus: http.StatusNotFound, wantCode: "instrument_not_found"},
		{name: "instrument lookup failed", instrumentErr: storageErr, wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
		{name: "market lookup failed", marketErr: storageErr, wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			previousLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
			t.Cleanup(func() { slog.SetDefault(previousLogger) })

			price, accrued, duration := 1000.0, 10.0, 3.0
			instrument := models.Instrument{
				ID: uuid.New(), ISIN: "RU000A0JX0J2", Name: "Test bond",
				InstrumentType: "bond", IssuerName: "Test issuer", Currency: "RUB", FaceValue: 1000,
			}
			store := &instrumentTestStore{
				instrument: instrument, instrumentErr: tt.instrumentErr, marketErr: tt.marketErr,
				marketData: models.MarketData{
					InstrumentID: instrument.ID, TradeDate: time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC),
					PriceClean: &price, AccruedInt: &accrued, DurationMod: &duration,
				},
			}
			isin := tt.isin
			if isin == "" {
				isin = instrument.ISIN
			}
			handler := NewHandler(service.NewService(store))
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v1/instruments/{isin}", handler.GetInstrument)
			request := httptest.NewRequest(http.MethodGet, "/api/v1/instruments/"+isin+tt.query, nil)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, tt.wantStatus, response.Body.String())
			}
			if response.Header().Get("Content-Type") != "application/json" {
				t.Error("response must have application/json content type")
			}
			if tt.wantCode != "" {
				var body map[string]json.RawMessage
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				var code, message string
				if err := json.Unmarshal(body["code"], &code); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(body["message"], &message); err != nil {
					t.Fatal(err)
				}
				if code != tt.wantCode || message == "" || string(body["details"]) != "null" || len(body) != 3 {
					t.Errorf("unexpected error response: %s", response.Body.String())
				}
				if strings.Contains(response.Body.String(), storageErr.Error()) {
					t.Error("response exposes the internal storage error")
				}
				if tt.wantStatus == http.StatusInternalServerError {
					if !strings.Contains(logs.String(), "get instrument failed") || !strings.Contains(logs.String(), storageErr.Error()) {
						t.Error("internal error was not logged")
					}
				}
				if tt.wantStatus == http.StatusBadRequest && (store.lookupCalls != 0 || store.marketCalls != 0) {
					t.Error("invalid request must be rejected before accessing storage")
				}
				if tt.instrumentErr != nil && store.marketCalls != 0 {
					t.Error("market data must not be queried after an instrument lookup error")
				}
				return
			}

			var body GetInstrumentResponse
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.ID != instrument.ID || body.ISIN != instrument.ISIN || body.Name != instrument.Name || body.Currency != instrument.Currency {
				t.Error("response contains incorrect instrument data")
			}
			if store.lookupCalls != 1 || store.marketCalls != 1 || store.source != tt.wantSource {
				t.Error("incorrect storage calls or selected source")
			}
			if errors.Is(tt.marketErr, gorm.ErrRecordNotFound) {
				if body.ReadyForCalculation || body.PriceClean != nil || body.AccruedInt != nil || body.DurationMod != nil || body.MarketDate != nil {
					t.Error("missing market data must produce an unready instrument with null values")
				}
				if len(body.MissingParameters) != 1 || body.MissingParameters[0] != "market_data" {
					t.Errorf("unexpected missing parameters: %v", body.MissingParameters)
				}
			} else {
				if !body.ReadyForCalculation || body.MissingParameters == nil || len(body.MissingParameters) != 0 {
					t.Error("complete market data must produce a ready instrument and an empty missing parameters array")
				}
				if body.Source != string(tt.wantSource) || body.PriceClean == nil || *body.PriceClean != price || body.AccruedInt == nil || *body.AccruedInt != accrued || body.DurationMod == nil || *body.DurationMod != duration {
					t.Error("response contains incorrect market data")
				}
				if body.MarketDate == nil || !body.MarketDate.Equal(store.marketData.TradeDate) {
					t.Error("response contains an incorrect market date")
				}
			}
		})
	}
}
