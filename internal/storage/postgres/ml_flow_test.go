package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/api"
	"github.com/iriski-app/backend/internal/clients/ml"
	mlv1 "github.com/iriski-app/backend/internal/clients/ml/v1"
	"github.com/iriski-app/backend/internal/clients/moex"
	"github.com/iriski-app/backend/internal/collector"
	"github.com/iriski-app/backend/internal/service"
	"github.com/iriski-app/backend/internal/storage/models"
	"google.golang.org/grpc"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type flowMOEX struct{ *ingestionClientFake }

func (c *flowMOEX) GetMarketData(ctx context.Context, board string, date time.Time) ([]moex.MarketData, error) {
	data, err := c.ingestionClientFake.GetMarketData(ctx, board, date)
	for i := range data {
		data[i].AccruedInt = new(10.0)
		data[i].MaturityDate = "2027-02-03"
		data[i].BuybackDateAvailable = true
	}
	return data, err
}

type flowML struct {
	mlv1.UnimplementedMLServiceServer
	calls []mlv1.MetricKind
}

func (s *flowML) EstimateMissingMetric(_ context.Context, req *mlv1.EstimateMissingMetricRequest) (*mlv1.EstimateMissingMetricResponse, error) {
	s.calls = append(s.calls, req.Metric)
	value := 2.5
	if req.Metric == mlv1.MetricKind_METRIC_KIND_CONVEXITY {
		value = 4.0
	}
	return &mlv1.EstimateMissingMetricResponse{RequestId: req.RequestId, InstrumentId: req.InstrumentId, AsOfDate: req.AsOfDate, Metric: req.Metric, Value: value, Source: mlv1.MetricSource_METRIC_SOURCE_CATBOOST, ModelVersion: "test-v1", ReviewRequired: true}, nil
}

func TestMOEXToMLCalculationFlow(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not set")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	adminSQL, err := admin.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := adminSQL.Close(); err != nil {
			t.Error(err)
		}
	}()
	schema := "ml_flow_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Error(err)
		}
	}()
	testDSN := dsn + " search_path=" + schema
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		query := parsed.Query()
		query.Set("search_path", schema)
		parsed.RawQuery = query.Encode()
		testDSN = parsed.String()
	}
	db, err := gorm.Open(postgres.Open(testDSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := CreateTables(db); err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	date := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	result, collectErr := collector.NewService(store, &flowMOEX{&ingestionClientFake{bondizationErr: errors.New("missing schedule")}}).Run(context.Background(), date)
	if collectErr == nil || result.RowsLoaded != 1 {
		t.Fatalf("collector result=%+v error=%v", result, collectErr)
	}
	instrument, err := store.GetInstrument(context.Background(), "RU000A0JS3W6")
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	grpcServer := grpc.NewServer()
	mock := &flowML{}
	mlv1.RegisterMLServiceServer(grpcServer, mock)
	go func() { _ = grpcServer.Serve(listener) }()
	defer grpcServer.Stop()
	client, err := ml.NewClient(listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	}()
	svc := service.NewService(store)
	svc.ML = client
	router := api.NewRouter(api.NewHandler(svc))

	for _, tc := range []struct {
		name                string
		duration, convexity any
		wantCalls           int
		wantStatus          int
		old                 bool
		offline             bool
	}{
		{"complete", 2.0, 3.0, 0, http.StatusCreated, false, false},
		{"missing duration", nil, 3.0, 1, http.StatusCreated, false, false},
		{"missing convexity", 2.0, nil, 1, http.StatusCreated, false, false},
		{"both missing", nil, nil, 2, http.StatusCreated, false, false},
		{"ML unavailable", nil, nil, 0, http.StatusServiceUnavailable, false, true},
		{"old snapshot", nil, nil, 0, http.StatusUnprocessableEntity, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := db.Model(&models.MarketData{}).Where("instrument_id = ? AND trade_date = ?", instrument.ID, date).Updates(map[string]any{"duration_mod": tc.duration, "convexity": tc.convexity}).Error; err != nil {
				t.Fatal(err)
			}
			if tc.old {
				if err := db.Where("instrument_id = ? AND trade_date = ?", instrument.ID, date).Delete(&models.BondFeatures{}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if tc.offline {
				grpcServer.Stop()
			}
			mock.calls = nil
			body := fmt.Sprintf(`{"positions":[{"isin":%q,"side":"buy","quantity":2}],"incomplete":true}`, instrument.ISIN)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/risk-calculations", strings.NewReader(body)))
			if response.Code != tc.wantStatus || len(mock.calls) != tc.wantCalls {
				t.Fatalf("status=%d body=%s calls=%v", response.Code, response.Body.String(), mock.calls)
			}
			if tc.wantStatus != http.StatusCreated {
				return
			}
			var got api.CalculationResponse
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			duration, convexity := 2.0, 3.0
			if tc.duration == nil {
				duration = 2.5
			}
			if tc.convexity == nil {
				convexity = 4
			}
			want := 2 * 1000.0 * (-duration*0.0001 + 0.5*convexity*0.0001*0.0001)
			if got.TotalDV01Rub == nil || math.Abs(*got.TotalDV01Rub-want) > 1e-8 || len(got.Positions) != 1 || got.Incomplete == nil || !*got.Incomplete {
				t.Fatalf("calculation=%+v want=%v", got, want)
			}
			if (len(got.Positions[0].DurationEstimate) > 0) != (tc.duration == nil) || (len(got.Positions[0].ConvexityEstimate) > 0) != (tc.convexity == nil) {
				t.Fatalf("missing metadata: %+v", got.Positions[0])
			}
			cookies := response.Result().Cookies()
			get := httptest.NewRequest(http.MethodGet, "/api/v1/risk-calculations/"+got.ID.String(), nil)
			for _, cookie := range cookies {
				get.AddCookie(cookie)
			}
			saved := httptest.NewRecorder()
			router.ServeHTTP(saved, get)
			if saved.Code != http.StatusOK || !strings.Contains(saved.Body.String(), `"incomplete":true`) || (tc.wantCalls > 0 && !strings.Contains(saved.Body.String(), "test-v1")) {
				t.Fatalf("saved status=%d body=%s", saved.Code, saved.Body.String())
			}
		})
	}
}
