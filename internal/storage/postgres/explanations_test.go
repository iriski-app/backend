package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
)

func TestSaveVerifiedExplanation(t *testing.T) {
	store := newIngestionStore(t)
	if err := CreateTables(store.db); err != nil {
		t.Fatal(err)
	}
	run := models.CalcRun{ID: uuid.New(), SessionID: uuid.New(), AsOfDate: time.Now(), Status: models.CalcStatusSuccess, CreatedAt: time.Now()}
	if err := store.db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	input := models.LLMExplanation{ID: uuid.New(), CalcRunID: run.ID, GeneratedBy: "v1", Answer: "DV01", ValidationOK: true, FactIDs: []byte(`["total_rub"]`), CreatedAt: time.Now()}
	if err := store.SaveExplanation(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	var stored models.LLMExplanation
	if err := store.db.First(&stored, "id = ?", input.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.ValidationOK || string(stored.FactIDs) != `["total_rub"]` || stored.CalcRunID != run.ID {
		t.Fatalf("stored %+v", stored)
	}
}
