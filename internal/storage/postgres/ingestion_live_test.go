package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/clients/moex"
	"github.com/iriski-app/backend/internal/collector"
	"github.com/iriski-app/backend/internal/service"
	"github.com/iriski-app/backend/internal/storage/models"
)

type singleBondMOEX struct {
	*moex.Client
	secID string
}

type liveCalculationStore struct {
	*Store
	saved bool
}

func (s *liveCalculationStore) SaveCalculation(ctx context.Context, run models.CalcRun, results []models.CalcResult) error {
	s.saved = true
	return nil
}

func (c *singleBondMOEX) GetInstruments(ctx context.Context, board string) ([]moex.Instrument, error) {
	if board != "TQOB" {
		return nil, nil
	}
	instruments, err := c.Client.GetInstruments(ctx, board)
	if err != nil {
		return nil, err
	}
	for _, instrument := range instruments {
		if instrument.SECID == c.secID {
			return []moex.Instrument{instrument}, nil
		}
	}
	return nil, fmt.Errorf("instrument %s not found", c.secID)
}

func (c *singleBondMOEX) GetMarketData(ctx context.Context, board string, date time.Time) ([]moex.MarketData, error) {
	if board != "TQOB" {
		return nil, nil
	}
	marketData, err := c.Client.GetMarketData(ctx, board, date)
	if err != nil {
		return nil, err
	}
	for _, data := range marketData {
		if data.SECID == c.secID {
			return []moex.MarketData{data}, nil
		}
	}
	return nil, fmt.Errorf("market data for %s not found", c.secID)
}

func TestCollectorRunLiveMOEX(t *testing.T) {
	if os.Getenv("TEST_MOEX_LIVE") != "1" {
		t.Skip("TEST_MOEX_LIVE is not set")
	}
	store := newIngestionStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := &singleBondMOEX{Client: moex.NewClient("", 30*time.Second), secID: "SU26238RMFS4"}
	date := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	result, err := collector.NewService(store, client).Run(ctx, date)
	if err != nil || result.Status != models.ETLStatusSuccess || result.RowsLoaded != 1 {
		t.Fatalf("collector result: %+v, error: %v", result, err)
	}
	instrument, err := store.GetInstrument(ctx, "RU000A1038V6")
	if err != nil {
		t.Fatal(err)
	}
	marketData, err := store.GetMarketData(ctx, instrument.ID, models.SourceMoex, date)
	if err != nil {
		t.Fatal(err)
	}
	if marketData.Convexity == nil || *marketData.Convexity <= 0 {
		t.Fatalf("convexity was not stored: %+v", marketData)
	}
	calcStore := &liveCalculationStore{Store: store}
	riskService := service.NewService(calcStore)
	info, err := riskService.GetInstrument(ctx, instrument.ISIN, models.SourceMoex)
	if err != nil || !info.ReadyForCalculation {
		t.Fatalf("instrument is not ready: %+v, error: %v", info, err)
	}
	run, results, err := riskService.CalculatePositions(ctx, uuid.New(), []service.PositionInput{{InstrumentID: instrument.ID, Side: models.DealSideBuy, Quantity: 1}})
	if err != nil || run.TotalDV01Rub == nil || *run.TotalDV01Rub >= 0 || len(results) != 1 || results[0].Convexity == nil || !calcStore.saved {
		t.Fatalf("risk calculation: %+v, results: %+v, error: %v", run, results, err)
	}
	t.Logf("stored convexity %.8f for %s on %s", *marketData.Convexity, client.secID, date.Format("2006-01-02"))
	t.Logf("calculated signed DV01 %.8f RUB", *run.TotalDV01Rub)
}
