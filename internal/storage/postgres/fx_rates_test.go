package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm"
)

func TestFXRatesSaveAndFind(t *testing.T) {
	store := newIngestionStore(t)
	if err := store.db.AutoMigrate(&models.FXRate{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	date := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	rate := models.FXRate{Currency: "USD", EffectiveDate: date, Source: "cbr", Nominal: 1, Value: "80.123456", LoadedAt: date}
	if err := store.SaveFXRates(ctx, []models.FXRate{rate}); err != nil {
		t.Fatal(err)
	}
	rate.Value = "81.123456"
	if err := store.SaveFXRates(ctx, []models.FXRate{rate}); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetFXRate(ctx, "USD", "cbr", date.AddDate(0, 0, 1))
	if err != nil || got.Value != "81.123456000000" {
		t.Fatalf("unexpected updated rate: %+v, error: %v", got, err)
	}
	var count int64
	if err := store.db.Model(&models.FXRate{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("unexpected rate count %d, error: %v", count, err)
	}
	_, err = store.GetFXRate(ctx, "USD", "cbr", date.AddDate(0, 0, -1))
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected no future rate, got %v", err)
	}
}
