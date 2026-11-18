package service

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestMarketDateCutoff(t *testing.T) {
	cases := []struct {
		now  time.Time
		want string
	}{
		{time.Date(2026, 10, 25, 20, 59, 0, 0, time.UTC), "2026-10-24"},
		{time.Date(2026, 10, 25, 21, 0, 0, 0, time.UTC), "2026-10-25"},
		{time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), "2025-12-31"},
	}
	for _, tc := range cases {
		if got := marketDateCutoff(tc.now).Format("2006-01-02"); got != tc.want {
			t.Errorf("cutoff for %s = %s, want %s", tc.now, got, tc.want)
		}
	}
}

func TestMissingMarketDate(t *testing.T) {
	store := &fakeStore{
		GetInstrumentFunc: func(context.Context, string) (models.Instrument, error) {
			return models.Instrument{ID: uuid.New()}, nil
		},
		GetLatestMarketDateFunc: func(context.Context, models.Source, time.Time) (time.Time, error) {
			return time.Time{}, gorm.ErrRecordNotFound
		},
	}
	svc := NewService(store)
	info, err := svc.GetInstrument(context.Background(), "RU000A0JX0J2", models.SourceMoex)
	if err != nil || info.ReadyForCalculation || info.MarketData != nil || len(info.MissingParameters) != 1 || info.MissingParameters[0] != "market_data" {
		t.Fatalf("info = %+v, error = %v", info, err)
	}
	_, _, err = svc.CalculatePositions(context.Background(), uuid.New(), []PositionInput{{InstrumentID: uuid.New(), Side: models.DealSideBuy, Quantity: 1}})
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("got %v, want record not found", err)
	}
}
