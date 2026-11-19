package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
)

func TestSaveCalculationWithEstimate(t *testing.T) {
	store := newIngestionStore(t)
	if err := CreateTables(store.db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	instrument, err := store.UpsertInstrument(ctx, ingestionInstrument())
	if err != nil {
		t.Fatal(err)
	}
	run := models.CalcRun{ID: uuid.New(), SessionID: uuid.New(), AsOfDate: time.Now(), Status: models.CalcStatusSuccess, CreatedAt: time.Now()}
	metadata := `{"source":"catboost","model_version":"v1","review_required":true}`
	result := models.CalcResult{ID: uuid.New(), CalcRunID: run.ID, InstrumentID: instrument.ID, Side: models.DealSideBuy, Quantity: 1, Currency: "RUB", ConvexityEstimate: &metadata}
	if err := store.db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.db.Create(&result).Error; err != nil {
		t.Fatal(err)
	}
	_, saved, err := store.GetCalculation(ctx, run.SessionID, run.ID)
	if err != nil || len(saved) != 1 || saved[0].ConvexityEstimate == nil || !strings.Contains(*saved[0].ConvexityEstimate, `"model_version": "v1"`) {
		t.Fatalf("saved=%+v error=%v", saved, err)
	}
}
