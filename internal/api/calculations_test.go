package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/service"
	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm"
)

type calculationTestStore struct {
	service.Storage
	instruments  map[string]models.Instrument
	marketData   map[uuid.UUID]models.MarketData
	isinErr      error
	idErr        error
	marketErr    error
	saveErr      error
	saveCalls    int
	savedRun     models.CalcRun
	savedResults []models.CalcResult
	getErr       error
	getCalls     int
}

func (s *calculationTestStore) GetInstrument(_ context.Context, isin string) (models.Instrument, error) {
	if s.isinErr != nil {
		return models.Instrument{}, s.isinErr
	}
	instrument, ok := s.instruments[isin]
	if !ok {
		return models.Instrument{}, gorm.ErrRecordNotFound
	}
	return instrument, nil
}

func (s *calculationTestStore) GetInstrumentByID(_ context.Context, id uuid.UUID) (models.Instrument, error) {
	if s.idErr != nil {
		return models.Instrument{}, s.idErr
	}
	for _, instrument := range s.instruments {
		if instrument.ID == id {
			return instrument, nil
		}
	}
	return models.Instrument{}, gorm.ErrRecordNotFound
}

func (s *calculationTestStore) GetMarketData(_ context.Context, id uuid.UUID, _ models.Source, _ time.Time) (models.MarketData, error) {
	if s.marketErr != nil {
		return models.MarketData{}, s.marketErr
	}
	data, ok := s.marketData[id]
	if !ok {
		return models.MarketData{}, gorm.ErrRecordNotFound
	}
	return data, nil
}

func (s *calculationTestStore) SaveCalculation(_ context.Context, run models.CalcRun, results []models.CalcResult) error {
	s.saveCalls++
	s.savedRun = run
	s.savedResults = append([]models.CalcResult(nil), results...)
	return s.saveErr
}

func (s *calculationTestStore) GetCalculation(_ context.Context, sessionID uuid.UUID, calcID uuid.UUID) (models.CalcRun, []models.CalcResult, error) {
	s.getCalls++
	if s.getErr != nil {
		return models.CalcRun{}, nil, s.getErr
	}
	if s.savedRun.ID != calcID || s.savedRun.SessionID != sessionID {
		return models.CalcRun{}, nil, gorm.ErrRecordNotFound
	}
	return s.savedRun, s.savedResults, nil
}

func newCalculationTestStore() (*calculationTestStore, []Position) {
	positions := []Position{
		{ISIN: "RU000A0JX0J2", Side: models.DealSideBuy, Quantity: 10},
		{ISIN: "US0378331005", Side: models.DealSideSell, Quantity: 5},
	}
	store := &calculationTestStore{
		instruments: make(map[string]models.Instrument),
		marketData:  make(map[uuid.UUID]models.MarketData),
	}
	accrued := []float64{10, 20}
	durations := []float64{3, 2}
	for i, position := range positions {
		instrument := models.Instrument{ID: uuid.New(), ISIN: position.ISIN, Currency: "RUB"}
		price := 1000.0
		store.instruments[position.ISIN] = instrument
		store.marketData[instrument.ID] = models.MarketData{
			InstrumentID: instrument.ID,
			Source:       models.SourceMoex,
			TradeDate:    time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC),
			PriceClean:   &price,
			AccruedInt:   &accrued[i],
			DurationMod:  &durations[i],
		}
	}
	return store, positions
}

func serveCalculation(t *testing.T, store *calculationTestStore, body string, sessionCookie *string) *httptest.ResponseRecorder {
	t.Helper()
	defer func() {
		if value := recover(); value != nil {
			t.Fatalf("calculation handler panicked: %v", value)
		}
	}()
	handler := NewHandler(service.NewService(store))
	mux := NewRouter(handler)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/risk-calculations", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if sessionCookie != nil {
		request.AddCookie(&http.Cookie{Name: "session_id", Value: *sessionCookie})
	}
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}

func TestCalculationSuccess(t *testing.T) {
	tests := []struct {
		name       string
		newSession bool
		count      int
		wantTotal  float64
	}{
		{name: "single new session", newSession: true, count: 1, wantTotal: 3.03},
		{name: "single existing session", count: 1, wantTotal: 3.03},
		{name: "multiple positions new session", newSession: true, count: 2, wantTotal: 2.01},
		{name: "multiple positions existing session", count: 2, wantTotal: 2.01},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, positions := newCalculationTestStore()
			positions = positions[:tt.count]
			body, err := json.Marshal(CalculationRequest{Positions: positions})
			if err != nil {
				t.Fatal(err)
			}
			sessionID := uuid.New()
			cookieValue := sessionID.String()
			var cookie *string
			if !tt.newSession {
				cookie = &cookieValue
			}
			response := serveCalculation(t, store, string(body), cookie)
			if response.Code != http.StatusCreated {
				t.Fatalf("status = %d, want 201; body: %s", response.Code, response.Body.String())
			}
			if response.Header().Get("Content-Type") != "application/json" {
				t.Error("response must have application/json content type")
			}
			var result CalculationResponse
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.ID == uuid.Nil || result.ID != store.savedRun.ID || result.Status != models.CalcStatusSuccess {
				t.Error("response has incorrect calculation ID or status")
			}
			if result.AsOfDate.IsZero() || !result.AsOfDate.Equal(store.savedRun.AsOfDate) {
				t.Error("response has an incorrect calculation date")
			}
			assertCalculationNumber(t, "total_dv01_rub", result.TotalDV01Rub, tt.wantTotal)
			assertCalculationNumber(t, "saved total_dv01_rub", store.savedRun.TotalDV01Rub, tt.wantTotal)
			if store.saveCalls != 1 || len(store.savedResults) != tt.count || len(result.Positions) != tt.count {
				t.Fatalf("save calls = %d, saved positions = %d, response positions = %d; want 1, %d, %d", store.saveCalls, len(store.savedResults), len(result.Positions), tt.count, tt.count)
			}
			wantDV01 := []float64{3.03, -1.02}
			wantValues := []float64{10100, 5100}
			for i, position := range result.Positions {
				input, saved := positions[i], store.savedResults[i]
				data := store.marketData[store.instruments[input.ISIN].ID]
				if position.ID == uuid.Nil || position.ID != saved.ID || saved.CalcRunID != result.ID {
					t.Errorf("position %d has incorrect result or calculation ID", i)
				}
				if position.InstrumentID != data.InstrumentID || position.Side != input.Side || position.Quantity != input.Quantity {
					t.Errorf("position %d does not match the request", i)
				}
				assertCalculationNumber(t, "market_value", position.MarketValue, wantValues[i])
				assertCalculationNumber(t, "duration_mod", position.DurationMod, *data.DurationMod)
				assertCalculationNumber(t, "dv01", position.DV01, wantDV01[i])
				if !reflect.DeepEqual(position.MarketValue, saved.FairValue) || !reflect.DeepEqual(position.DV01, saved.DV01) {
					t.Errorf("position %d differs from the saved result", i)
				}
				if position.Source != data.Source || position.MarketDate == nil || !position.MarketDate.Equal(data.TradeDate) {
					t.Errorf("position %d has an incorrect data source or market date", i)
				}
			}
			httpResponse := response.Result()
			defer func() { _ = httpResponse.Body.Close() }()
			cookies := httpResponse.Cookies()
			if tt.newSession {
				if len(cookies) != 1 || cookies[0].Name != "session_id" {
					t.Fatal("new session must set exactly one session_id cookie")
				}
				createdID, err := uuid.Parse(cookies[0].Value)
				if err != nil || createdID == uuid.Nil || createdID != store.savedRun.SessionID {
					t.Fatal("saved session ID does not match the new cookie")
				}
				if !cookies[0].HttpOnly || cookies[0].Path != "/" || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].MaxAge <= 0 {
					t.Error("incorrect session cookie attributes")
				}
			} else if store.savedRun.SessionID != sessionID || len(cookies) != 0 {
				t.Error("existing session must be reused without replacing its cookie")
			}
		})
	}
}

func TestCalculationInvalidRequest(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{name: "empty body", body: "", wantStatus: 400, wantCode: "invalid_json"},
		{name: "malformed JSON", body: `{"positions":`, wantStatus: 400, wantCode: "invalid_json"},
		{name: "wrong positions type", body: `{"positions":"invalid"}`, wantStatus: 400, wantCode: "invalid_json"},
		{name: "fractional quantity", body: `{"positions":[{"isin":"RU000A0JX0J2","side":"buy","quantity":1.5}]}`, wantStatus: 400, wantCode: "invalid_json"},
		{name: "missing positions", body: `{}`, wantStatus: 400, wantCode: "invalid_parameter"},
		{name: "empty positions", body: `{"positions":[]}`, wantStatus: 400, wantCode: "invalid_parameter"},
		{name: "zero quantity", body: `{"positions":[{"isin":"RU000A0JX0J2","side":"buy","quantity":0}]}`, wantStatus: 400, wantCode: "invalid_parameter"},
		{name: "negative quantity", body: `{"positions":[{"isin":"RU000A0JX0J2","side":"buy","quantity":-1}]}`, wantStatus: 400, wantCode: "invalid_parameter"},
		{name: "invalid side", body: `{"positions":[{"isin":"RU000A0JX0J2","side":"invalid","quantity":1}]}`, wantStatus: 400, wantCode: "invalid_parameter"},
		{name: "invalid ISIN", body: `{"positions":[{"isin":"invalid","side":"buy","quantity":1}]}`, wantStatus: 400, wantCode: "invalid_isin"},
		{name: "invalid check digit", body: `{"positions":[{"isin":"RU000A0JX0J3","side":"buy","quantity":1}]}`, wantStatus: 400, wantCode: "invalid_isin"},
		{name: "unknown instrument", body: `{"positions":[{"isin":"GB0002634946","side":"buy","quantity":1}]}`, wantStatus: 404, wantCode: "instrument_not_found"},
		{
			name:       "invalid second position",
			body:       `{"positions":[{"isin":"RU000A0JX0J2","side":"buy","quantity":10},{"isin":"US0378331005","side":"sell","quantity":0}]}`,
			wantStatus: 400, wantCode: "invalid_parameter",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, _ := newCalculationTestStore()
			session := uuid.NewString()
			response := serveCalculation(t, store, tt.body, &session)
			assertCalculationHTTPError(t, response, tt.wantStatus, tt.wantCode)
			if store.saveCalls != 0 {
				t.Error("invalid request must not save a calculation")
			}
		})
	}
}

func TestCalculationInvalidSession(t *testing.T) {
	for _, session := range []string{"", "invalid-uuid"} {
		t.Run(fmt.Sprintf("cookie=%q", session), func(t *testing.T) {
			store := &calculationTestStore{}
			response := serveCalculation(t, store, `{"positions":[]}`, &session)
			assertCalculationHTTPError(t, response, http.StatusBadRequest, "session_error")
		})
	}
}

func TestCalculationDataErrors(t *testing.T) {
	storageErr := errors.New("internal database failure")
	tests := []struct {
		name       string
		missing    string
		isinErr    error
		idErr      error
		marketErr  error
		saveErr    error
		wantStatus int
		wantCode   string
		wantSaves  int
	}{
		{name: "missing price", missing: "price", wantStatus: 422, wantCode: "missing_parameter"},
		{name: "missing accrued interest", missing: "accrued", wantStatus: 422, wantCode: "missing_parameter"},
		{name: "missing duration", missing: "duration", wantStatus: 422, wantCode: "missing_duration"},
		{name: "missing market data", marketErr: gorm.ErrRecordNotFound, wantStatus: 422, wantCode: "calculation_data_not_found"},
		{name: "wrapped missing market data", marketErr: fmt.Errorf("lookup: %w", gorm.ErrRecordNotFound), wantStatus: 422, wantCode: "calculation_data_not_found"},
		{name: "instrument disappears", idErr: gorm.ErrRecordNotFound, wantStatus: 422, wantCode: "calculation_data_not_found"},
		{name: "instrument lookup error", isinErr: storageErr, wantStatus: 500, wantCode: "internal_error"},
		{name: "instrument ID lookup error", idErr: storageErr, wantStatus: 500, wantCode: "internal_error"},
		{name: "market data lookup error", marketErr: storageErr, wantStatus: 500, wantCode: "internal_error"},
		{name: "save error", saveErr: storageErr, wantStatus: 500, wantCode: "internal_error", wantSaves: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, positions := newCalculationTestStore()
			store.isinErr, store.idErr, store.marketErr, store.saveErr = tt.isinErr, tt.idErr, tt.marketErr, tt.saveErr
			instrumentID := store.instruments[positions[0].ISIN].ID
			data := store.marketData[instrumentID]
			switch tt.missing {
			case "price":
				data.PriceClean = nil
			case "accrued":
				data.AccruedInt = nil
			case "duration":
				data.DurationMod = nil
			}
			store.marketData[instrumentID] = data
			body, err := json.Marshal(CalculationRequest{Positions: positions[:1]})
			if err != nil {
				t.Fatal(err)
			}
			session := uuid.NewString()
			response := serveCalculation(t, store, string(body), &session)
			assertCalculationHTTPError(t, response, tt.wantStatus, tt.wantCode)
			if store.saveCalls != tt.wantSaves {
				t.Errorf("SaveCalculation called %d times, want %d", store.saveCalls, tt.wantSaves)
			}
			if strings.Contains(response.Body.String(), storageErr.Error()) {
				t.Error("response exposes internal database error details")
			}
		})
	}
}

func assertCalculationHTTPError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, status, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Error("error response must have application/json content type")
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var result ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Code != code || result.Message == "" || len(body) != 3 || string(body["details"]) != "null" {
		t.Errorf("unexpected error response: %s", response.Body.String())
	}
}

func assertCalculationNumber(t *testing.T, field string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is nil, want %.12f", field, want)
	}
	if math.IsNaN(*got) || math.Abs(*got-want) > 1e-9 {
		t.Errorf("%s = %.12f, want %.12f", field, *got, want)
	}
}

func TestGetCalculation(t *testing.T) {
	storageErr := errors.New("internal calculation lookup failure")
	tests := []struct {
		name          string
		missingCookie bool
		invalidCookie bool
		emptyCookie   bool
		invalidID     bool
		unknownID     bool
		foreign       bool
		getErr        error
		wantStatus    int
		wantCode      string
		wantCalls     int
	}{
		{name: "saved calculation", wantStatus: 200, wantCalls: 1},
		{name: "missing session", missingCookie: true, wantStatus: 404, wantCode: "calculation_not_found"},
		{name: "invalid session", invalidCookie: true, wantStatus: 400, wantCode: "session_error"},
		{name: "empty session", emptyCookie: true, wantStatus: 400, wantCode: "session_error"},
		{name: "invalid calculation ID", invalidID: true, wantStatus: 400, wantCode: "invalid_calculation_id"},
		{name: "unknown calculation", unknownID: true, wantStatus: 404, wantCode: "calculation_not_found", wantCalls: 1},
		{name: "foreign session", foreign: true, wantStatus: 404, wantCode: "calculation_not_found", wantCalls: 1},
		{name: "wrapped not found", getErr: fmt.Errorf("lookup: %w", gorm.ErrRecordNotFound), wantStatus: 404, wantCode: "calculation_not_found", wantCalls: 1},
		{name: "storage failure", getErr: storageErr, wantStatus: 500, wantCode: "internal_error", wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &calculationTestStore{
				savedRun: models.CalcRun{
					ID: uuid.New(), SessionID: uuid.New(), Status: models.CalcStatusSuccess,
					AsOfDate: time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC),
				},
				getErr: tt.getErr,
			}
			calcID := store.savedRun.ID.String()
			if tt.invalidID {
				calcID = "invalid-uuid"
			} else if tt.unknownID {
				calcID = uuid.NewString()
			}
			session := store.savedRun.SessionID.String()
			if tt.invalidCookie {
				session = "invalid-uuid"
			} else if tt.emptyCookie {
				session = ""
			} else if tt.foreign {
				session = uuid.NewString()
			}
			handler := NewHandler(service.NewService(store))
			mux := NewRouter(handler)
			request := httptest.NewRequest(http.MethodGet, "/api/v1/risk-calculations/"+calcID, nil)
			if !tt.missingCookie {
				request.AddCookie(&http.Cookie{Name: "session_id", Value: session})
			}
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)

			if store.getCalls != tt.wantCalls {
				t.Errorf("GetCalculation called %d times, want %d", store.getCalls, tt.wantCalls)
			}
			if store.saveCalls != 0 || response.Header().Get("Set-Cookie") != "" {
				t.Error("GET must not save calculations or create a session")
			}
			if tt.wantCode != "" {
				assertCalculationHTTPError(t, response, tt.wantStatus, tt.wantCode)
				if strings.Contains(response.Body.String(), storageErr.Error()) {
					t.Error("response exposes internal storage error details")
				}
				return
			}
			if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("unexpected response: status=%d, body=%s", response.Code, response.Body.String())
			}
			var result CalculationResponse
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.ID != store.savedRun.ID || result.Status != store.savedRun.Status || !result.AsOfDate.Equal(store.savedRun.AsOfDate) {
				t.Error("response differs from the saved calculation")
			}
			if result.Positions == nil || len(result.Positions) != 0 {
				t.Error("empty results must be serialized as an empty array")
			}
		})
	}
}

func TestCalculationCreateThenGetSnapshot(t *testing.T) {
	store, positions := newCalculationTestStore()
	handler := NewHandler(service.NewService(store))
	mux := NewRouter(handler)
	body, err := json.Marshal(CalculationRequest{Positions: positions})
	if err != nil {
		t.Fatal(err)
	}
	createRequest := httptest.NewRequest(http.MethodPost, "/api/v1/risk-calculations", strings.NewReader(string(body)))
	created := httptest.NewRecorder()
	mux.ServeHTTP(created, createRequest)
	if created.Code != http.StatusCreated {
		t.Fatalf("creation status = %d; body: %s", created.Code, created.Body.String())
	}
	httpResponse := created.Result()
	defer func() { _ = httpResponse.Body.Close() }()
	cookies := httpResponse.Cookies()
	if len(cookies) != 1 {
		t.Fatal("creation must return a session cookie")
	}

	store.instruments = nil
	store.marketData = nil
	for i := 0; i < 2; i++ {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/risk-calculations/"+store.savedRun.ID.String(), nil)
		request.AddCookie(cookies[0])
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("lookup status = %d; body: %s", response.Code, response.Body.String())
		}
		if response.Body.String() != created.Body.String() {
			t.Errorf("saved snapshot differs from creation response:\ncreated: %s\nloaded: %s", created.Body.String(), response.Body.String())
		}
		if strings.Contains(response.Body.String(), store.savedRun.SessionID.String()) {
			t.Error("response must not expose the session ID")
		}
		if response.Header().Get("Set-Cookie") != "" {
			t.Error("GET must preserve the existing session")
		}
	}
	if store.saveCalls != 1 || store.getCalls != 2 {
		t.Errorf("save calls = %d, get calls = %d; want 1, 2", store.saveCalls, store.getCalls)
	}
}
