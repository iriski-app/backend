package collector

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/clients/moex"
	"github.com/iriski-app/backend/internal/storage/models"
)

func TestRunSuccess(t *testing.T) {
	store := &collectorStoreFake{}
	client := newCollectorClientFake()
	addCollectorPosition(client, "TQOB", "BOND1", "RU000A0JS3W6")
	addCollectorPosition(client, "TQCB", "BOND2", "RU000A0JTK38")
	client.history["TQCB"][0].Duration = nil
	store.onSave = func() {
		if len(client.calls) != 4 {
			t.Errorf("saving started before all requests completed: %v", client.calls)
		}
	}
	date := time.Date(2026, 10, 26, 19, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	result, err := NewService(store, client).Run(context.Background(), date)
	if err != nil {
		t.Fatal(err)
	}
	if result.ID == uuid.Nil || result.ID != store.run.ID || result.Status != models.ETLStatusSuccess || result.RowsLoaded != 2 || result.RowsRejected != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if store.createCalls != 1 || store.finishCalls != 1 || store.saveCalls != 2 || store.errorMessage != "" {
		t.Fatalf("unexpected store calls: %+v", store)
	}
	if store.run.Source != models.SourceMoex || store.run.TradeDate.Format("2006-01-02") != "2026-10-26" || store.run.TradeDate.Hour() != 0 {
		t.Fatalf("unexpected run: %+v", store.run)
	}
	for _, date := range client.dates {
		if date.Format("2006-01-02") != "2026-10-26" || date.Hour() != 0 || date.Location() != time.UTC {
			t.Fatalf("unexpected requested date: %v", date)
		}
	}
	for _, record := range store.saved {
		if record.instrument.ID == uuid.Nil || record.marketData.InstrumentID != record.instrument.ID || record.instrument.Currency != "RUB" {
			t.Fatalf("unexpected saved instrument: %+v", record)
		}
		assertNumber(t, "price", record.marketData.PriceClean, 986.4)
		if record.marketData.Source != models.SourceMoex || record.marketData.LoadedAt.IsZero() {
			t.Fatalf("unexpected saved market data: %+v", record.marketData)
		}
	}
	assertNumber(t, "duration", store.saved[0].marketData.DurationMod, 1.6)
	if store.saved[1].marketData.DurationMod != nil {
		t.Fatal("missing duration was replaced")
	}
}

func TestRunEmptyDate(t *testing.T) {
	store := &collectorStoreFake{}
	client := newCollectorClientFake()
	result, err := NewService(store, client).Run(context.Background(), collectorTradeDate())
	if err != nil || result.Status != models.ETLStatusSuccess || result.RowsLoaded != 0 || result.RowsRejected != 0 {
		t.Fatalf("unexpected result: %+v, error: %v", result, err)
	}
	if store.saveCalls != 0 || store.finishCalls != 1 || len(client.calls) != 4 {
		t.Fatalf("unexpected calls: store %+v, client %v", store, client.calls)
	}
}

func TestRunSkipsUnsupportedRows(t *testing.T) {
	tests := []struct {
		name   string
		change func(*collectorClientFake)
	}{
		{
			name: "foreign currency",
			change: func(client *collectorClientFake) {
				client.instruments["TQOB"][0].FaceUnit = "USD"
				client.history["TQOB"][0].FaceUnit = "USD"
			},
		},
		{
			name: "floating coupon",
			change: func(client *collectorClientFake) {
				client.instruments["TQOB"][0].BondType = "Облигация с плавающим купоном"
				client.history["TQOB"][0].BondType = "Облигация с плавающим купоном"
			},
		},
		{
			name: "no trades",
			change: func(client *collectorClientFake) {
				client.history["TQOB"][0].NumTrades = testInt(0)
			},
		},
		{
			name: "no trades without reference",
			change: func(client *collectorClientFake) {
				client.history["TQOB"][0].NumTrades = testInt(0)
				client.instruments["TQOB"] = nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &collectorStoreFake{}
			client := newCollectorClientFake()
			addCollectorPosition(client, "TQOB", "BOND1", "RU000A0JS3W6")
			tt.change(client)
			result, err := NewService(store, client).Run(context.Background(), collectorTradeDate())
			if err != nil || result.Status != models.ETLStatusSuccess || result.RowsLoaded != 0 || result.RowsRejected != 0 {
				t.Fatalf("unexpected result: %+v, error: %v", result, err)
			}
			if store.saveCalls != 0 || store.errorMessage != "" {
				t.Fatalf("unsupported row was treated as an error: %+v", store)
			}
		})
	}
}

func TestRunRejectsInvalidRows(t *testing.T) {
	tests := []struct {
		name   string
		change func(*collectorClientFake)
	}{
		{name: "missing reference", change: func(client *collectorClientFake) { client.instruments["TQCB"] = nil }},
		{name: "duplicate reference", change: func(client *collectorClientFake) {
			client.instruments["TQCB"] = append(client.instruments["TQCB"], client.instruments["TQCB"][0])
		}},
		{name: "invalid ISIN", change: func(client *collectorClientFake) { client.instruments["TQCB"][0].ISIN = "invalid" }},
		{name: "missing nominal", change: func(client *collectorClientFake) { client.instruments["TQCB"][0].FaceValue = nil }},
		{name: "wrong date", change: func(client *collectorClientFake) { client.history["TQCB"][0].TradeDate = "2026-10-25" }},
		{name: "wrong history board", change: func(client *collectorClientFake) { client.history["TQCB"][0].BoardID = "TQOB" }},
		{name: "wrong reference board", change: func(client *collectorClientFake) { client.instruments["TQCB"][0].BoardID = "TQOB" }},
		{name: "missing number of trades", change: func(client *collectorClientFake) { client.history["TQCB"][0].NumTrades = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &collectorStoreFake{}
			client := newCollectorClientFake()
			addCollectorPosition(client, "TQOB", "BOND1", "RU000A0JS3W6")
			addCollectorPosition(client, "TQCB", "BOND2", "RU000A0JTK38")
			tt.change(client)
			result, err := NewService(store, client).Run(context.Background(), collectorTradeDate())
			if !errors.Is(err, ErrInvalidData) || result.Status != models.ETLStatusPartialSuccess || result.RowsLoaded != 1 || result.RowsRejected != 1 {
				t.Fatalf("unexpected result: %+v, error: %v", result, err)
			}
			if store.saveCalls != 1 || store.saved[0].instrument.ISIN != "RU000A0JS3W6" || store.errorMessage == "" {
				t.Fatalf("unexpected saved rows: %+v", store)
			}
		})
	}
}

func TestRunRejectsDuplicateMarketData(t *testing.T) {
	for _, name := range []string{"trading sessions", "same board", "different boards"} {
		t.Run(name, func(t *testing.T) {
			store := &collectorStoreFake{}
			client := newCollectorClientFake()
			addCollectorPosition(client, "TQOB", "BOND1", "RU000A0JS3W6")
			switch name {
			case "trading sessions":
				other := client.history["TQOB"][0]
				other.TradingSession = testInt(1)
				client.history["TQOB"] = append(client.history["TQOB"], other)
			case "same board":
				addCollectorPosition(client, "TQOB", "BOND2", "RU000A0JS3W6")
			case "different boards":
				addCollectorPosition(client, "TQCB", "BOND2", "RU000A0JS3W6")
			}
			result, err := NewService(store, client).Run(context.Background(), collectorTradeDate())
			if !errors.Is(err, ErrInvalidData) || result.Status != models.ETLStatusFailed || result.RowsLoaded != 0 || result.RowsRejected != 2 {
				t.Fatalf("unexpected duplicate result: %+v, error: %v", result, err)
			}
			if store.saveCalls != 0 {
				t.Fatal("ambiguous market data was saved")
			}
		})
	}
}

func TestRunSourceErrors(t *testing.T) {
	sourceErr := errors.New("source unavailable")
	tests := []struct {
		name       string
		change     func(*collectorClientFake)
		wantStatus models.ETLStatus
		wantLoaded int
		wantErr    error
	}{
		{
			name: "reference error",
			change: func(client *collectorClientFake) {
				client.instrumentErrors["TQCB"] = sourceErr
			},
			wantStatus: models.ETLStatusPartialSuccess, wantLoaded: 1, wantErr: sourceErr,
		},
		{
			name: "history timeout",
			change: func(client *collectorClientFake) {
				client.historyErrors["TQCB"] = context.DeadlineExceeded
			},
			wantStatus: models.ETLStatusPartialSuccess, wantLoaded: 1, wantErr: context.DeadlineExceeded,
		},
		{
			name: "all boards failed",
			change: func(client *collectorClientFake) {
				client.instrumentErrors["TQOB"] = sourceErr
				client.historyErrors["TQCB"] = sourceErr
			},
			wantStatus: models.ETLStatusFailed, wantLoaded: 0, wantErr: sourceErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &collectorStoreFake{}
			client := newCollectorClientFake()
			addCollectorPosition(client, "TQOB", "BOND1", "RU000A0JS3W6")
			addCollectorPosition(client, "TQCB", "BOND2", "RU000A0JTK38")
			tt.change(client)
			result, err := NewService(store, client).Run(context.Background(), collectorTradeDate())
			if !errors.Is(err, tt.wantErr) || result.Status != tt.wantStatus || result.RowsLoaded != tt.wantLoaded || result.RowsRejected != 0 {
				t.Fatalf("unexpected source error result: %+v, error: %v", result, err)
			}
			if store.finishCalls != 1 || store.errorMessage == "" {
				t.Fatalf("run was not finished with error: %+v", store)
			}
		})
	}
}

func TestRunStorageError(t *testing.T) {
	storeErr := errors.New("database unavailable")
	for _, failedCall := range []int{1, 2} {
		store := &collectorStoreFake{saveErrors: map[int]error{failedCall: storeErr}}
		client := newCollectorClientFake()
		addCollectorPosition(client, "TQOB", "BOND1", "RU000A0JS3W6")
		addCollectorPosition(client, "TQCB", "BOND2", "RU000A0JTK38")
		addCollectorPosition(client, "TQCB", "BOND3", "RU000A0JVW48")
		result, err := NewService(store, client).Run(context.Background(), collectorTradeDate())
		wantStatus := models.ETLStatusFailed
		if failedCall == 2 {
			wantStatus = models.ETLStatusPartialSuccess
		}
		if !errors.Is(err, storeErr) || result.Status != wantStatus || result.RowsLoaded != failedCall-1 || result.RowsRejected != 1 {
			t.Fatalf("unexpected storage error result: %+v, error: %v", result, err)
		}
		if store.saveCalls != failedCall || store.finishCalls != 1 {
			t.Fatalf("writing continued after database error: %+v", store)
		}
	}
}

func TestRunInstrumentWithoutQuotes(t *testing.T) {
	for _, emptyHistory := range []bool{false, true} {
		store := &collectorStoreFake{}
		client := newCollectorClientFake()
		addCollectorPosition(client, "TQOB", "BOND1", "RU000A0JS3W6")
		if emptyHistory {
			client.history["TQOB"] = nil
		} else {
			client.history["TQOB"][0].NumTrades = testInt(0)
		}
		result, err := NewService(store, client).Run(context.Background(), collectorTradeDate())
		if err != nil || result.Status != models.ETLStatusSuccess || result.RowsLoaded != 0 || result.RowsRejected != 0 {
			t.Fatalf("unexpected reference result: %+v, error: %v", result, err)
		}
		if store.instrumentCalls != 1 || store.saveCalls != 0 {
			t.Fatalf("instrument without quotes was not saved: %+v", store)
		}
	}
}

func TestRunInstrumentStorageError(t *testing.T) {
	storeErr := errors.New("cannot save reference")
	store := &collectorStoreFake{instrumentErr: storeErr}
	client := newCollectorClientFake()
	addCollectorPosition(client, "TQOB", "BOND1", "RU000A0JS3W6")
	client.history["TQOB"] = nil
	result, err := NewService(store, client).Run(context.Background(), collectorTradeDate())
	if !errors.Is(err, storeErr) || result.Status != models.ETLStatusFailed || result.RowsLoaded != 0 || result.RowsRejected != 0 || store.finishCalls != 1 {
		t.Fatalf("unexpected reference error: %+v, error: %v", result, err)
	}
}

func TestRunStartErrors(t *testing.T) {
	service := NewService(nil, nil)
	result, err := service.Run(context.Background(), time.Time{})
	if !errors.Is(err, ErrInvalidData) || result.ID != uuid.Nil {
		t.Fatalf("unexpected invalid date result: %+v, error: %v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err = service.Run(ctx, collectorTradeDate())
	if !errors.Is(err, context.Canceled) || result.ID != uuid.Nil {
		t.Fatalf("unexpected cancellation result: %+v, error: %v", result, err)
	}
	storeErr := errors.New("cannot create run")
	store := &collectorStoreFake{createErr: storeErr}
	client := newCollectorClientFake()
	result, err = NewService(store, client).Run(context.Background(), collectorTradeDate())
	if !errors.Is(err, storeErr) || result.ID != uuid.Nil || store.finishCalls != 0 || len(client.calls) != 0 {
		t.Fatalf("unexpected create error result: %+v, error: %v", result, err)
	}
}

func TestRunCancellation(t *testing.T) {
	for _, afterSave := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		store := &collectorStoreFake{}
		client := newCollectorClientFake()
		addCollectorPosition(client, "TQOB", "BOND1", "RU000A0JS3W6")
		addCollectorPosition(client, "TQCB", "BOND2", "RU000A0JTK38")
		wantStatus := models.ETLStatusFailed
		wantLoaded := 0
		if afterSave {
			store.onSave = cancel
			wantStatus = models.ETLStatusPartialSuccess
			wantLoaded = 1
		} else {
			store.onCreate = cancel
		}
		result, err := NewService(store, client).Run(ctx, collectorTradeDate())
		cancel()
		if !errors.Is(err, context.Canceled) || result.Status != wantStatus || result.RowsLoaded != wantLoaded || result.RowsRejected != 0 {
			t.Fatalf("unexpected canceled result: %+v, error: %v", result, err)
		}
		if store.finishCalls != 1 || store.finishContextError != nil || store.finishDeadline.IsZero() {
			t.Fatalf("run could not be finalized after cancellation: %+v", store)
		}
		if time.Until(store.finishDeadline) > 5*time.Second {
			t.Fatalf("unexpected finalization deadline: %v", store.finishDeadline)
		}
	}
}

func TestRunFinishError(t *testing.T) {
	finishErr := errors.New("cannot finish run")
	for _, sourceFailed := range []bool{false, true} {
		store := &collectorStoreFake{finishErr: finishErr}
		client := newCollectorClientFake()
		sourceErr := errors.New("source unavailable")
		if sourceFailed {
			client.instrumentErrors["TQOB"] = sourceErr
		} else {
			addCollectorPosition(client, "TQOB", "BOND1", "RU000A0JS3W6")
		}
		result, err := NewService(store, client).Run(context.Background(), collectorTradeDate())
		if !errors.Is(err, finishErr) || result.Status != models.ETLStatusRunning || result.ID == uuid.Nil || store.finishCalls != 1 {
			t.Fatalf("unexpected finish failure: %+v, error: %v", result, err)
		}
		if sourceFailed && !errors.Is(err, sourceErr) {
			t.Fatalf("source error was lost: %v", err)
		}
	}
}

type collectorClientFake struct {
	instruments      map[string][]moex.Instrument
	history          map[string][]moex.MarketData
	instrumentErrors map[string]error
	historyErrors    map[string]error
	calls            []string
	dates            []time.Time
}

func (c *collectorClientFake) GetInstruments(ctx context.Context, board string) ([]moex.Instrument, error) {
	c.calls = append(c.calls, "instruments "+board)
	return c.instruments[board], c.instrumentErrors[board]
}

func (c *collectorClientFake) GetMarketData(ctx context.Context, board string, date time.Time) ([]moex.MarketData, error) {
	c.calls = append(c.calls, "history "+board)
	c.dates = append(c.dates, date)
	return c.history[board], c.historyErrors[board]
}

func newCollectorClientFake() *collectorClientFake {
	return &collectorClientFake{
		instruments:      make(map[string][]moex.Instrument),
		history:          make(map[string][]moex.MarketData),
		instrumentErrors: make(map[string]error),
		historyErrors:    make(map[string]error),
	}
}

type collectorStoreFake struct {
	run                models.ETLRun
	createErr          error
	finishErr          error
	instrumentErr      error
	saveErrors         map[int]error
	saved              []marketDataRecord
	createCalls        int
	saveCalls          int
	finishCalls        int
	instrumentCalls    int
	errorMessage       string
	finishContextError error
	finishDeadline     time.Time
	onCreate           func()
	onSave             func()
}

func (s *collectorStoreFake) CreateETLRun(ctx context.Context, source models.Source, date time.Time) (models.ETLRun, error) {
	s.createCalls++
	if s.createErr != nil {
		return models.ETLRun{}, s.createErr
	}
	s.run = models.ETLRun{ID: uuid.New(), Source: source, TradeDate: date, Status: models.ETLStatusRunning}
	if s.onCreate != nil {
		s.onCreate()
	}
	return s.run, nil
}

func (s *collectorStoreFake) SaveMarketData(ctx context.Context, instrument models.Instrument, data models.MarketData) error {
	s.saveCalls++
	if err := s.saveErrors[s.saveCalls]; err != nil {
		return err
	}
	s.saved = append(s.saved, marketDataRecord{instrument: instrument, marketData: &data})
	if s.onSave != nil {
		s.onSave()
	}
	return nil
}

func (s *collectorStoreFake) UpsertInstrument(ctx context.Context, instrument models.Instrument) (models.Instrument, error) {
	s.instrumentCalls++
	return instrument, s.instrumentErr
}

func (s *collectorStoreFake) FinishETLRun(ctx context.Context, id uuid.UUID, status models.ETLStatus, loaded int, rejected int, message string) error {
	s.finishCalls++
	s.finishContextError = ctx.Err()
	s.finishDeadline, _ = ctx.Deadline()
	if s.finishErr != nil {
		return s.finishErr
	}
	if id != s.run.ID {
		return errors.New("unexpected ETL run ID")
	}
	s.run.Status = status
	s.run.RowsLoaded = loaded
	s.run.RowsRejected = rejected
	s.errorMessage = message
	return nil
}

func addCollectorPosition(client *collectorClientFake, board string, secID string, isin string) {
	instrument := testInstrument()
	instrument.BoardID = board
	instrument.SECID = secID
	instrument.ISIN = isin
	data := testMarketData()
	data.BoardID = board
	data.SECID = secID
	client.instruments[board] = append(client.instruments[board], instrument)
	client.history[board] = append(client.history[board], data)
}

func collectorTradeDate() time.Time {
	return time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
}

var _ Client = (*moex.Client)(nil)
var _ Client = (*collectorClientFake)(nil)
var _ Storage = (*collectorStoreFake)(nil)

func TestRunAllRowsInvalid(t *testing.T) {
	store := &collectorStoreFake{}
	client := newCollectorClientFake()
	addCollectorPosition(client, "TQOB", "BOND1", "RU000A0JS3W6")
	client.history["TQOB"][0].TradeDate = "2026-10-25"
	result, err := NewService(store, client).Run(context.Background(), collectorTradeDate())
	if !errors.Is(err, ErrInvalidData) || result.Status != models.ETLStatusFailed || result.RowsLoaded != 0 || result.RowsRejected != 1 {
		t.Fatalf("unexpected invalid data result: %+v, error: %v", result, err)
	}
	if !strings.Contains(store.errorMessage, "unexpected board or date") {
		t.Fatalf("missing rejection reason: %s", store.errorMessage)
	}
}
