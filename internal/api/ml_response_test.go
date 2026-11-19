package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/iriski-app/backend/internal/storage/models"
)

func TestCalculationResponseIncludesEstimateMetadata(t *testing.T) {
	metadata := `{"source":"catboost","model_version":"v1","review_required":true}`
	response := newCalculationResponse(models.CalcRun{}, []models.CalcResult{{DurationEstimate: &metadata}})
	data, err := json.Marshal(response)
	if err != nil || !strings.Contains(string(data), `"duration_estimate":{"source":"catboost"`) || strings.Contains(string(data), `"convexity_estimate"`) {
		t.Fatalf("unexpected response %s: %v", data, err)
	}
}
