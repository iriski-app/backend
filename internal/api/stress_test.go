package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/service"
	"github.com/iriski-app/backend/internal/storage/models"
)

func TestStressScenarioHTTP(t *testing.T) {
	store, _ := newCalculationTestStore()
	date := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	value, duration, convexity := 1000.0, 2.0, 3.0
	sessionID, calcID := uuid.New(), uuid.New()
	store.savedRun = models.CalcRun{ID: calcID, SessionID: sessionID, Status: models.CalcStatusSuccess, AsOfDate: date}
	store.savedResults = []models.CalcResult{{ID: uuid.New(), FairValue: &value, DurationMod: &duration, Convexity: &convexity, MarketDate: &date, Currency: "RUB", Side: models.DealSideBuy}}
	router := NewRouter(NewHandler(service.NewService(store)))
	for _, tc := range []struct {
		body   string
		status int
	}{{`{"shift_bp":0}`, 200}, {`{"shift_bp":100}`, 200}, {`{}`, 400}, {`{"shift_bp":201}`, 422}} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/risk-calculations/"+calcID.String()+"/stress-scenarios", strings.NewReader(tc.body))
		req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID.String()})
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != tc.status {
			t.Fatalf("body %s status %d response %s", tc.body, res.Code, res.Body.String())
		}
	}
}
