package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	mlv1 "github.com/iriski-app/backend/internal/clients/ml/v1"
	"github.com/iriski-app/backend/internal/service"
	"github.com/iriski-app/backend/internal/storage/models"
)

type explanationAPIStore struct {
	*calculationTestStore
	saved int
}

func (s *explanationAPIStore) SaveExplanation(_ context.Context, value models.LLMExplanation) error {
	s.saved++
	return nil
}

type explanationAPIML struct{ verified bool }

func (m *explanationAPIML) EstimateMissingMetric(context.Context, *mlv1.EstimateMissingMetricRequest) (*mlv1.EstimateMissingMetricResponse, error) {
	return nil, nil
}
func (m *explanationAPIML) ExplainCalculation(_ context.Context, r *mlv1.ExplainCalculationRequest) (*mlv1.ExplainCalculationResponse, error) {
	return &mlv1.ExplainCalculationResponse{RequestId: r.RequestId, CalculationId: r.CalculationId, Text: "DV01", Verified: m.verified, ModelVersion: "v1", FactIds: []string{"total_rub"}}, nil
}

func TestExplainCalculationHTTP(t *testing.T) {
	base, _ := newCalculationTestStore()
	store := &explanationAPIStore{calculationTestStore: base}
	id, session := uuid.New(), uuid.New()
	total := 0.1
	store.savedRun = models.CalcRun{ID: id, SessionID: session, Status: models.CalcStatusSuccess, AsOfDate: time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC), TotalDV01Rub: &total}
	store.savedResults = []models.CalcResult{{ID: uuid.New(), Side: models.DealSideBuy, DV01: &total}}
	client := &explanationAPIML{verified: true}
	svc := service.NewService(store)
	svc.ML = client
	router := NewRouter(NewHandler(svc))
	for _, tc := range []struct {
		session  uuid.UUID
		verified bool
		status   int
	}{{session, true, 201}, {uuid.New(), true, 404}, {session, false, 503}} {
		client.verified = tc.verified
		req := httptest.NewRequest(http.MethodPost, "/api/v1/risk-calculations/"+id.String()+"/explanations", strings.NewReader(""))
		req.AddCookie(&http.Cookie{Name: "session_id", Value: tc.session.String()})
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != tc.status {
			t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
		}
	}
	if store.saved != 1 {
		t.Fatalf("saved=%d", store.saved)
	}
}
