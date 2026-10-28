package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/clients/moex"
	"github.com/iriski-app/backend/internal/collector"
	"github.com/iriski-app/backend/internal/storage/models"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestUpsertInstrumentPreservesID(t *testing.T) {
	store := newIngestionStore(t)
	ctx := context.Background()
	instrument, err := store.UpsertInstrument(ctx, ingestionInstrument())
	if err != nil {
		t.Fatal(err)
	}
	if instrument.ID == uuid.Nil {
		t.Fatal("instrument ID is empty")
	}
	marketData := ingestionMarketData(instrument.ID)
	if err := store.UpsertMarketData(ctx, marketData); err != nil {
		t.Fatal(err)
	}
	update := ingestionInstrument()
	update.ID = uuid.New()
	update.Name = "Updated bond"
	update.FaceValue = 900
	update.CouponFreq = 0
	update.CouponRate = nil
	update.MaturityDate = nil
	update.IssuerName = ""
	update.Sector = ""
	update.Rating = ""
	saved, err := store.UpsertInstrument(ctx, update)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID != instrument.ID {
		t.Fatalf("instrument ID changed from %s to %s", instrument.ID, saved.ID)
	}
	if saved.Name != "Updated bond" || saved.FaceValue != 900 || saved.CouponFreq != 0 || saved.CouponRate != nil || saved.MaturityDate != nil {
		t.Fatalf("fields were not updated: %+v", saved)
	}
	if saved.IssuerName != instrument.IssuerName || saved.Sector != instrument.Sector || saved.Rating != instrument.Rating {
		t.Fatalf("additional instrument data was lost: %+v", saved)
	}
	stored, err := store.GetInstrument(ctx, instrument.ISIN)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != saved.ID || stored.Name != saved.Name || stored.CouponRate != nil || stored.CouponFreq != 0 {
		t.Fatalf("stored instrument differs: %+v", stored)
	}
	var count int64
	if err := store.db.Model(&models.Instrument{}).Where("isin = ?", instrument.ISIN).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("got %d instruments, want 1", count)
	}
	if _, err := store.GetMarketData(ctx, instrument.ID, marketData.Source, marketData.TradeDate); err != nil {
		t.Fatalf("market data no longer references instrument: %v", err)
	}
	saved.IssuerName = "New issuer"
	saved.Sector = "New sector"
	saved.Rating = "AA"
	saved, err = store.UpsertInstrument(ctx, saved)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID != instrument.ID || saved.IssuerName != "New issuer" || saved.Sector != "New sector" || saved.Rating != "AA" {
		t.Fatalf("additional fields were not updated: %+v", saved)
	}
}

func TestUpsertMarketDataKeysAndMissingValues(t *testing.T) {
	store := newIngestionStore(t)
	ctx := context.Background()
	instrument, err := store.UpsertInstrument(ctx, ingestionInstrument())
	if err != nil {
		t.Fatal(err)
	}
	data := ingestionMarketData(instrument.ID)
	if err := store.UpsertMarketData(ctx, data); err != nil {
		t.Fatal(err)
	}
	otherSource := ingestionMarketData(instrument.ID)
	otherSource.Source = models.SourceRuData
	if err := store.UpsertMarketData(ctx, otherSource); err != nil {
		t.Fatal(err)
	}
	otherDate := ingestionMarketData(instrument.ID)
	otherDate.TradeDate = data.TradeDate.AddDate(0, 0, 1)
	if err := store.UpsertMarketData(ctx, otherDate); err != nil {
		t.Fatal(err)
	}
	zero := 0.0
	zeroTrades := 0
	data.PriceClean = nil
	data.AccruedInt = &zero
	data.DurationMod = nil
	data.Convexity = nil
	data.YTM = nil
	data.NumTrades = &zeroTrades
	data.LoadedAt = data.LoadedAt.Add(time.Hour)
	if err := store.UpsertMarketData(ctx, data); err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetMarketData(ctx, instrument.ID, data.Source, data.TradeDate)
	if err != nil {
		t.Fatal(err)
	}
	if stored.PriceClean != nil || stored.DurationMod != nil || stored.Convexity != nil || stored.YTM != nil {
		t.Fatalf("missing values were not updated: %+v", stored)
	}
	if stored.AccruedInt == nil || *stored.AccruedInt != 0 || stored.NumTrades == nil || *stored.NumTrades != 0 {
		t.Fatalf("zero values were not updated: %+v", stored)
	}
	if !stored.LoadedAt.Equal(data.LoadedAt) {
		t.Fatalf("load time %v, want %v", stored.LoadedAt, data.LoadedAt)
	}
	for _, expected := range []models.MarketData{otherSource, otherDate} {
		stored, err := store.GetMarketData(ctx, instrument.ID, expected.Source, expected.TradeDate)
		if err != nil {
			t.Fatal(err)
		}
		if stored.PriceClean == nil || *stored.PriceClean != *expected.PriceClean || stored.DurationMod == nil || *stored.DurationMod != *expected.DurationMod {
			t.Fatalf("another source or date was changed: %+v", stored)
		}
	}
	var count int64
	if err := store.db.Model(&models.MarketData{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("got %d market data rows, want 3", count)
	}
}

func TestUpsertMarketDataDoesNotCreateInstrument(t *testing.T) {
	store := newIngestionStore(t)
	instrument := ingestionInstrument()
	instrument.ID = uuid.New()
	data := ingestionMarketData(instrument.ID)
	data.Instrument = instrument
	err := store.UpsertMarketData(context.Background(), data)
	var dbErr *pgconn.PgError
	if !errors.As(err, &dbErr) || dbErr.Code != "23503" {
		t.Fatalf("got %v, want foreign key violation", err)
	}
}

func TestSaveMarketDataPreservesInstrumentID(t *testing.T) {
	store := newIngestionStore(t)
	ctx := context.Background()
	existing, err := store.UpsertInstrument(ctx, ingestionInstrument())
	if err != nil {
		t.Fatal(err)
	}
	instrument := ingestionInstrument()
	instrument.ID = uuid.New()
	instrument.Name = "Updated bond"
	data := ingestionMarketData(instrument.ID)
	if err := store.SaveMarketData(ctx, instrument, data); err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetInstrument(ctx, instrument.ISIN)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != existing.ID || stored.Name != "Updated bond" {
		t.Fatalf("unexpected saved instrument: %+v", stored)
	}
	marketData, err := store.GetMarketData(ctx, existing.ID, data.Source, data.TradeDate)
	if err != nil {
		t.Fatal(err)
	}
	if marketData.InstrumentID != existing.ID || marketData.PriceClean == nil || *marketData.PriceClean != *data.PriceClean {
		t.Fatalf("market data uses wrong instrument: %+v", marketData)
	}
}

func TestSaveMarketDataRollback(t *testing.T) {
	for _, existing := range []bool{false, true} {
		store := newIngestionStore(t)
		ctx := context.Background()
		instrument := ingestionInstrument()
		if existing {
			if _, err := store.UpsertInstrument(ctx, instrument); err != nil {
				t.Fatal(err)
			}
		}
		instrument.ID = uuid.New()
		instrument.Name = "Must be rolled back"
		data := ingestionMarketData(instrument.ID)
		price := 10000000000000.0
		data.PriceClean = &price
		err := store.SaveMarketData(ctx, instrument, data)
		var dbErr *pgconn.PgError
		if !errors.As(err, &dbErr) || dbErr.Code != "22003" {
			t.Fatalf("got %v, want numeric overflow", err)
		}
		stored, err := store.GetInstrument(ctx, instrument.ISIN)
		if existing {
			if err != nil || stored.Name != "Test bond" {
				t.Fatalf("instrument update was not rolled back: %+v, error: %v", stored, err)
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("new instrument was not rolled back: %+v, error: %v", stored, err)
		}
		var count int64
		if err := store.db.Model(&models.MarketData{}).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("market data was not rolled back: %d rows", count)
		}
	}
}

func TestLargeBondPriceMigration(t *testing.T) {
	store := newIngestionStore(t)
	ctx := context.Background()
	if err := CreateTables(store.db); err != nil {
		t.Fatal(err)
	}
	if err := store.db.Exec("ALTER TABLE market_data ALTER COLUMN price_clean TYPE numeric(12,6)").Error; err != nil {
		t.Fatal(err)
	}
	if err := store.db.Exec("ALTER TABLE calc_results ALTER COLUMN price_clean TYPE numeric(12,6)").Error; err != nil {
		t.Fatal(err)
	}
	instrument, err := store.UpsertInstrument(ctx, ingestionInstrument())
	if err != nil {
		t.Fatal(err)
	}
	data := ingestionMarketData(instrument.ID)
	if err := store.UpsertMarketData(ctx, data); err != nil {
		t.Fatal(err)
	}
	run := models.CalcRun{
		ID:        uuid.New(),
		SessionID: uuid.New(),
		AsOfDate:  data.TradeDate,
		Status:    models.CalcStatusSuccess,
		CreatedAt: data.LoadedAt,
	}
	if err := store.db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := models.CalcResult{
		ID:           uuid.New(),
		CalcRunID:    run.ID,
		InstrumentID: instrument.ID,
		Side:         models.DealSideBuy,
		Quantity:     1,
		Currency:     "RUB",
		PriceClean:   data.PriceClean,
	}
	if err := store.db.Create(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	if err := CreateTables(store.db); err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetMarketData(ctx, instrument.ID, data.Source, data.TradeDate)
	if err != nil || stored.PriceClean == nil || *stored.PriceClean != 1000 {
		t.Fatalf("migration changed old market data: %+v, error: %v", stored, err)
	}
	var oldSnapshot models.CalcResult
	if err := store.db.First(&oldSnapshot, "id = ?", snapshot.ID).Error; err != nil {
		t.Fatal(err)
	}
	if oldSnapshot.PriceClean == nil || *oldSnapshot.PriceClean != 1000 {
		t.Fatalf("migration changed old snapshot: %+v", oldSnapshot)
	}
	price := 9200000.0
	data.PriceClean = &price
	if err := store.SaveMarketData(ctx, instrument, data); err != nil {
		t.Fatal(err)
	}
	stored, err = store.GetMarketData(ctx, instrument.ID, data.Source, data.TradeDate)
	if err != nil || stored.PriceClean == nil || *stored.PriceClean != price {
		t.Fatalf("large market price was not saved: %+v, error: %v", stored, err)
	}
	snapshot.ID = uuid.New()
	snapshot.PriceClean = &price
	if err := store.db.Create(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	var storedSnapshot models.CalcResult
	if err := store.db.First(&storedSnapshot, "id = ?", snapshot.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedSnapshot.PriceClean == nil || *storedSnapshot.PriceClean != price {
		t.Fatalf("large snapshot price was not saved: %+v", storedSnapshot)
	}
}

func TestCollectorRunStorage(t *testing.T) {
	store := newIngestionStore(t)
	ctx := context.Background()
	service := collector.NewService(store, &ingestionClientFake{})
	date := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	var instrumentID uuid.UUID
	var previousRunID uuid.UUID
	for i := 0; i < 2; i++ {
		result, err := service.Run(ctx, date)
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != models.ETLStatusSuccess || result.RowsLoaded != 1 || result.RowsRejected != 0 || result.ID == previousRunID {
			t.Fatalf("unexpected collector result: %+v", result)
		}
		previousRunID = result.ID
		instrument, err := store.GetInstrument(ctx, "RU000A0JS3W6")
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			instrumentID = instrument.ID
		} else if instrument.ID != instrumentID {
			t.Fatalf("instrument ID changed after rerun: %s", instrument.ID)
		}
		data, err := store.GetMarketData(ctx, instrument.ID, models.SourceMoex, date)
		if err != nil {
			t.Fatal(err)
		}
		if data.PriceClean == nil || *data.PriceClean != 990 || data.DurationMod == nil || *data.DurationMod != 1.6 {
			t.Fatalf("unexpected normalized market data: %+v", data)
		}
		var run models.ETLRun
		if err := store.db.First(&run, "id = ?", result.ID).Error; err != nil {
			t.Fatal(err)
		}
		if run.Status != models.ETLStatusSuccess || run.FinishedAt == nil || run.RowsLoaded != 1 || run.RowsRejected != 0 {
			t.Fatalf("unexpected saved run: %+v", run)
		}
	}
	var count int64
	if err := store.db.Model(&models.MarketData{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("rerun created duplicate market data: %d", count)
	}
}

func TestCollectorRunWithoutQuotes(t *testing.T) {
	store := newIngestionStore(t)
	ctx := context.Background()
	service := collector.NewService(store, &ingestionClientFake{noQuotes: true})
	date := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	result, err := service.Run(ctx, date)
	if err != nil || result.Status != models.ETLStatusSuccess || result.RowsLoaded != 0 {
		t.Fatalf("unexpected result: %+v, error: %v", result, err)
	}
	instrument, err := store.GetInstrument(ctx, "RU000A0JS3W6")
	if err != nil || instrument.ID == uuid.Nil {
		t.Fatalf("instrument was not saved: %+v, error: %v", instrument, err)
	}
	if _, err := store.GetMarketData(ctx, instrument.ID, models.SourceMoex, date); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("unexpected market data: %v", err)
	}
}

func TestETLRunLifecycle(t *testing.T) {
	tests := []struct {
		name         string
		status       models.ETLStatus
		rowsLoaded   int
		rowsRejected int
		errorMessage string
	}{
		{name: "success", status: models.ETLStatusSuccess, rowsLoaded: 10},
		{name: "partial success", status: models.ETLStatusPartialSuccess, rowsLoaded: 8, rowsRejected: 2, errorMessage: "two rows rejected"},
		{name: "failed", status: models.ETLStatusFailed, errorMessage: "source unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newIngestionStore(t)
			ctx := context.Background()
			tradeDate := time.Date(2026, 10, 26, 0, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
			run, err := store.CreateETLRun(ctx, models.SourceMoex, tradeDate)
			if err != nil {
				t.Fatal(err)
			}
			if run.ID == uuid.Nil || run.Status != models.ETLStatusRunning || run.StartedAt.IsZero() || run.FinishedAt != nil {
				t.Fatalf("unexpected new run: %+v", run)
			}
			var stored models.ETLRun
			if err := store.db.First(&stored, "id = ?", run.ID).Error; err != nil {
				t.Fatal(err)
			}
			if stored.TradeDate.Format("2006-01-02") != "2026-10-26" || stored.Source != models.SourceMoex || stored.Status != models.ETLStatusRunning {
				t.Fatalf("unexpected stored run: %+v", stored)
			}
			if stored.StartedAt.IsZero() || stored.FinishedAt != nil || stored.RowsLoaded != 0 || stored.RowsRejected != 0 || stored.Error != "" {
				t.Fatalf("unexpected initial run values: %+v", stored)
			}
			if err := store.FinishETLRun(ctx, run.ID, tt.status, tt.rowsLoaded, tt.rowsRejected, tt.errorMessage); err != nil {
				t.Fatal(err)
			}
			stored = models.ETLRun{}
			if err := store.db.First(&stored, "id = ?", run.ID).Error; err != nil {
				t.Fatal(err)
			}
			if stored.Status != tt.status || stored.RowsLoaded != tt.rowsLoaded || stored.RowsRejected != tt.rowsRejected || stored.Error != tt.errorMessage {
				t.Fatalf("unexpected finished run: %+v", stored)
			}
			if stored.FinishedAt == nil || stored.FinishedAt.Before(stored.StartedAt) {
				t.Fatalf("unexpected finish time: %+v", stored)
			}
			if err := store.FinishETLRun(ctx, run.ID, models.ETLStatusSuccess, 0, 0, ""); !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("finished run can be overwritten: %v", err)
			}
			var unchanged models.ETLRun
			if err := store.db.First(&unchanged, "id = ?", run.ID).Error; err != nil {
				t.Fatal(err)
			}
			if unchanged.Status != stored.Status || unchanged.RowsLoaded != stored.RowsLoaded || unchanged.Error != stored.Error || !unchanged.FinishedAt.Equal(*stored.FinishedAt) {
				t.Fatalf("finished run was changed: %+v", unchanged)
			}
		})
	}
}

func TestETLRunMigration(t *testing.T) {
	store := newIngestionStore(t)
	if err := store.db.Migrator().DropColumn(&models.ETLRun{}, "TradeDate"); err != nil {
		t.Fatal(err)
	}
	if err := store.db.Migrator().DropColumn(&models.ETLRun{}, "StartedAt"); err != nil {
		t.Fatal(err)
	}
	if err := CreateTables(store.db); err != nil {
		t.Fatal(err)
	}
	run, err := store.CreateETLRun(context.Background(), models.SourceMoex, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var stored models.ETLRun
	if err := store.db.First(&stored, "id = ?", run.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.TradeDate.IsZero() || stored.StartedAt.IsZero() {
		t.Fatalf("new fields were not saved: %+v", stored)
	}
}

func TestETLRunInvalidInput(t *testing.T) {
	store := newIngestionStore(t)
	ctx := context.Background()
	if _, err := store.CreateETLRun(ctx, models.SourceMoex, time.Time{}); err == nil {
		t.Fatal("empty trade date was accepted")
	}
	run, err := store.CreateETLRun(ctx, models.SourceMoex, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name         string
		status       models.ETLStatus
		rowsLoaded   int
		rowsRejected int
	}{
		{name: "running", status: models.ETLStatusRunning},
		{name: "unknown status", status: "unknown"},
		{name: "negative loaded count", status: models.ETLStatusFailed, rowsLoaded: -1},
		{name: "negative rejected count", status: models.ETLStatusFailed, rowsRejected: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := store.FinishETLRun(ctx, run.ID, tt.status, tt.rowsLoaded, tt.rowsRejected, ""); err == nil {
				t.Fatal("invalid final run values were accepted")
			}
		})
	}
	if err := store.FinishETLRun(ctx, uuid.New(), models.ETLStatusSuccess, 0, 0, ""); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("unexpected missing run error: %v", err)
	}
	var stored models.ETLRun
	if err := store.db.First(&stored, "id = ?", run.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != models.ETLStatusRunning || stored.FinishedAt != nil || stored.RowsLoaded != 0 || stored.RowsRejected != 0 {
		t.Fatalf("invalid input changed the run: %+v", stored)
	}
}

func newIngestionStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() {
		if err := tx.Rollback().Error; err != nil {
			t.Errorf("rollback test transaction: %v", err)
		}
	})
	schema := "moex_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := tx.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("SET LOCAL search_path TO " + schema).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.AutoMigrate(&models.Instrument{}, &models.MarketData{}, &models.ETLRun{}); err != nil {
		t.Fatal(err)
	}
	return NewStore(tx)
}

func ingestionInstrument() models.Instrument {
	rate := 8.15
	maturityDate := time.Date(2027, 2, 3, 0, 0, 0, 0, time.UTC)
	return models.Instrument{
		ISIN:           "RU000A0JS3W6",
		Name:           "Test bond",
		InstrumentType: "bond",
		IssuerName:     "Test issuer",
		Currency:       "RUB",
		FaceValue:      1000,
		CouponType:     "fixed",
		CouponFreq:     2,
		Sector:         "Finance",
		Rating:         "AAA",
		CouponRate:     &rate,
		MaturityDate:   &maturityDate,
	}
}

func ingestionMarketData(instrumentID uuid.UUID) models.MarketData {
	price := 1000.0
	accrued := 12.5
	duration := 1.6
	convexity := 4.5
	yield := 12.0
	numTrades := 10
	return models.MarketData{
		InstrumentID: instrumentID,
		TradeDate:    time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC),
		Source:       models.SourceMoex,
		PriceType:    "trade",
		PriceClean:   &price,
		AccruedInt:   &accrued,
		DurationMod:  &duration,
		Convexity:    &convexity,
		YTM:          &yield,
		NumTrades:    &numTrades,
		LoadedAt:     time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC),
	}
}

type ingestionClientFake struct {
	noQuotes bool
}

func (c *ingestionClientFake) GetInstruments(ctx context.Context, board string) ([]moex.Instrument, error) {
	if board != "TQOB" {
		return nil, nil
	}
	instrument := ingestionInstrument()
	period := 182
	return []moex.Instrument{{
		SECID:         "BOND1",
		ISIN:          instrument.ISIN,
		Name:          instrument.Name,
		BoardID:       board,
		FaceValue:     &instrument.FaceValue,
		FaceUnit:      "SUR",
		CurrencyID:    "SUR",
		CouponPeriod:  &period,
		CouponPercent: instrument.CouponRate,
		MaturityDate:  "2027-02-03",
		BondType:      "Облигация с фиксированным (известным) купоном",
		CouponDetails: "Фиксированный с известными купонами",
		FaceValueType: "Фиксированный",
	}}, nil
}

func (c *ingestionClientFake) GetMarketData(ctx context.Context, board string, date time.Time) ([]moex.MarketData, error) {
	if board != "TQOB" || c.noQuotes {
		return nil, nil
	}
	closePrice := 99.0
	nominal := 1000.0
	duration := 730.0
	yield := 25.0
	numTrades := 12
	return []moex.MarketData{{
		SECID:         "BOND1",
		BoardID:       board,
		TradeDate:     date.Format("2006-01-02"),
		Close:         &closePrice,
		FaceValue:     &nominal,
		FaceUnit:      "SUR",
		CurrencyID:    "SUR",
		Duration:      &duration,
		YieldClose:    &yield,
		NumTrades:     &numTrades,
		BondType:      "Облигация с фиксированным (известным) купоном",
		CouponDetails: "Фиксированный с известными купонами",
		FaceValueType: "Фиксированный",
	}}, nil
}

var _ collector.Storage = (*Store)(nil)
