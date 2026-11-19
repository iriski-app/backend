package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/clients/moex"
	"github.com/iriski-app/backend/internal/storage/models"
)

type Client interface {
	GetInstruments(ctx context.Context, board string) ([]moex.Instrument, error)
	GetMarketData(ctx context.Context, board string, date time.Time) ([]moex.MarketData, error)
	GetBondization(ctx context.Context, secID string) (moex.Bondization, error)
	GetSettlementCalendar(ctx context.Context, year int) ([]moex.SettlementCalendar, error)
}

type Storage interface {
	CreateETLRun(ctx context.Context, source models.Source, tradeDate time.Time) (models.ETLRun, error)
	UpsertInstrument(ctx context.Context, instrument models.Instrument) (models.Instrument, error)
	SaveMarketData(ctx context.Context, instrument models.Instrument, marketData models.MarketData) error
	FinishETLRun(ctx context.Context, id uuid.UUID, status models.ETLStatus, rowsLoaded int, rowsRejected int, errorMessage string) error
}

type Service struct {
	Store   Storage
	Client  Client
	Workers int
}

type Result struct {
	ID           uuid.UUID
	Status       models.ETLStatus
	RowsLoaded   int
	RowsRejected int
}

type marketDataRecord struct {
	instrument       models.Instrument
	marketData       *models.MarketData
	sourceInstrument moex.Instrument
	sourceMarketData moex.MarketData
}

type convexityResult struct {
	index int
	value float64
	err   error
}

func (s *Service) calculateConvexities(ctx context.Context, records []marketDataRecord, counts map[string]int, tradeDate time.Time) <-chan convexityResult {
	type settlementResult struct {
		date time.Time
		err  error
	}
	type settlementKey struct {
		currency string
		board    string
	}
	settlements := make(map[settlementKey]settlementResult)
	calendars := make(map[string]map[int]settlementCalendar)
	for _, record := range records {
		if record.marketData != nil && counts[record.instrument.ISIN] == 1 && record.instrument.CouponType != "floating" && record.sourceMarketData.YieldClose != nil && validNumber(*record.sourceMarketData.YieldClose) && *record.sourceMarketData.YieldClose > -100 {
			key := settlementKey{currency: normalizeCurrency(record.sourceMarketData.CurrencyID), board: record.sourceMarketData.BoardID}
			if _, ok := settlements[key]; !ok {
				if calendars[key.currency] == nil {
					calendars[key.currency] = make(map[int]settlementCalendar)
				}
				calculator := convexityCalculator{client: s.Client, calendars: calendars[key.currency], currency: key.currency, board: key.board}
				date, err := calculator.settlementDate(ctx, tradeDate)
				settlements[key] = settlementResult{date: date, err: err}
			}
		}
	}
	workers := s.Workers
	if workers <= 0 {
		workers = 5
	}
	if workers > len(records) {
		workers = len(records)
	}
	jobs := make(chan int)
	completed := make(chan convexityResult)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				record := records[index]
				key := settlementKey{currency: normalizeCurrency(record.sourceMarketData.CurrencyID), board: record.sourceMarketData.BoardID}
				settlement := settlements[key]
				calculator := convexityCalculator{client: s.Client}
				value, err := calculator.calculate(ctx, record.sourceInstrument, record.sourceMarketData, settlement.date, settlement.err)
				select {
				case completed <- convexityResult{index: index, value: value, err: err}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for i, record := range records {
			if record.marketData == nil || counts[record.instrument.ISIN] != 1 || record.instrument.CouponType == "floating" {
				continue
			}
			select {
			case jobs <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(completed)
	}()
	return completed
}

func NewService(store Storage, client Client) *Service {
	return &Service{
		Store:   store,
		Client:  client,
		Workers: 5,
	}
}

func (s *Service) Run(ctx context.Context, tradeDate time.Time) (Result, error) {
	if tradeDate.IsZero() {
		return Result{}, fmt.Errorf("%w: trade date is required", ErrInvalidData)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	year, month, day := tradeDate.Date()
	tradeDate = time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	run, err := s.Store.CreateETLRun(ctx, models.SourceMoex, tradeDate)
	if err != nil {
		return Result{}, fmt.Errorf("create ETL run: %w", err)
	}
	result := Result{
		ID:     run.ID,
		Status: models.ETLStatusRunning,
	}
	slog.InfoContext(ctx, "starting MOEX collection", "run_id", run.ID, "trade_date", tradeDate.Format("2006-01-02"))
	var records []marketDataRecord
	var runErr error
	for _, board := range []string{"TQOB", "TQCB", "TQOY"} {
		if err := ctx.Err(); err != nil {
			runErr = errors.Join(runErr, err)
			break
		}
		boardRecords, rejected, err := s.collectBoard(ctx, board, tradeDate)
		records = append(records, boardRecords...)
		result.RowsRejected += rejected
		runErr = errors.Join(runErr, err)
	}
	counts := make(map[string]int, len(records))
	var convexityErr error
	convexityFailures := 0
	for _, record := range records {
		if record.marketData != nil {
			counts[record.instrument.ISIN]++
		}
	}
	workersCtx, stopWorkers := context.WithCancel(ctx)
	calculations := s.calculateConvexities(workersCtx, records, counts, tradeDate)
	pending := make(map[int]convexityResult)
	for index, record := range records {
		if err := ctx.Err(); err != nil {
			runErr = errors.Join(runErr, err)
			break
		}
		if record.marketData == nil {
			if _, err := s.Store.UpsertInstrument(ctx, record.instrument); err != nil {
				runErr = errors.Join(runErr, fmt.Errorf("save instrument %s: %w", record.instrument.ISIN, err))
				break
			}
			continue
		}
		if counts[record.instrument.ISIN] > 1 {
			result.RowsRejected++
			runErr = errors.Join(runErr, fmt.Errorf("%w: duplicate market data for %s", ErrInvalidData, record.instrument.ISIN))
			continue
		}
		if record.instrument.CouponType == "floating" {
			convexity := 0.0
			record.marketData.Convexity = &convexity
			record.marketData.DurationMod = nil
			if record.instrument.CouponFreq != nil && *record.instrument.CouponFreq > 0 {
				duration := 1 / float64(*record.instrument.CouponFreq)
				record.marketData.DurationMod = &duration
			}
		} else {
			calculation, ok := pending[index]
			closed := false
			for !ok {
				var next convexityResult
				select {
				case next, ok = <-calculations:
					if !ok {
						closed = true
						break
					}
					pending[next.index] = next
					calculation, ok = pending[index]
				case <-ctx.Done():
					ok = false
				}
				if ctx.Err() != nil || closed {
					break
				}
			}
			if ctx.Err() != nil {
				runErr = errors.Join(runErr, ctx.Err())
				break
			}
			if closed {
				runErr = errors.Join(runErr, errors.New("convexity workers stopped before completing all jobs"))
				break
			}
			delete(pending, index)
			convexity, err := calculation.value, calculation.err
			if err != nil {
				if ctx.Err() != nil {
					runErr = errors.Join(runErr, ctx.Err())
					break
				}
				convexityFailures++
				if convexityErr == nil {
					convexityErr = fmt.Errorf("%s: %w", record.sourceMarketData.SECID, err)
				}
				slog.WarnContext(ctx, "convexity unavailable", "secid", record.sourceMarketData.SECID, "error", err)
			} else {
				record.marketData.Convexity = &convexity
			}
		}
		if err := s.Store.SaveMarketData(ctx, record.instrument, *record.marketData); err != nil {
			result.RowsRejected++
			runErr = errors.Join(runErr, fmt.Errorf("save market data for %s: %w", record.instrument.ISIN, err))
			break
		}
		result.RowsLoaded++
	}
	stopWorkers()
	for range calculations {
	}
	if convexityFailures > 0 {
		runErr = errors.Join(runErr, fmt.Errorf("convexity unavailable for %d instruments (first: %w)", convexityFailures, convexityErr))
	}
	status := models.ETLStatusSuccess
	errorMessage := ""
	if runErr != nil {
		status = models.ETLStatusFailed
		if result.RowsLoaded > 0 {
			status = models.ETLStatusPartialSuccess
		}
		errorMessage = runErr.Error()
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	err = s.Store.FinishETLRun(finishCtx, run.ID, status, result.RowsLoaded, result.RowsRejected, errorMessage)
	if err != nil {
		slog.ErrorContext(ctx, "failed to finish MOEX collection", "run_id", run.ID, "error", err)
		return result, errors.Join(runErr, fmt.Errorf("finish ETL run: %w", err))
	}
	result.Status = status
	if runErr != nil {
		slog.ErrorContext(ctx, "MOEX collection finished with errors", "run_id", run.ID, "status", status, "rows_loaded", result.RowsLoaded, "rows_rejected", result.RowsRejected, "error", runErr)
	} else {
		slog.InfoContext(ctx, "MOEX collection finished", "run_id", run.ID, "status", status, "rows_loaded", result.RowsLoaded, "rows_rejected", result.RowsRejected)
	}
	return result, runErr
}

func (s *Service) collectBoard(
	ctx context.Context,
	board string,
	tradeDate time.Time) ([]marketDataRecord, int, error) {
	instruments, err := s.Client.GetInstruments(ctx, board)
	if err != nil {
		return nil, 0, fmt.Errorf("get instruments for %s: %w", board, err)
	}
	history, err := s.Client.GetMarketData(ctx, board, tradeDate)
	if err != nil {
		return nil, 0, fmt.Errorf("get market data for %s: %w", board, err)
	}
	instrumentsBySECID := make(map[string]moex.Instrument, len(instruments))
	duplicates := make(map[string]bool)
	for _, instrument := range instruments {
		secID := strings.TrimSpace(instrument.SECID)
		if _, exists := instrumentsBySECID[secID]; exists {
			duplicates[secID] = true
		}
		instrumentsBySECID[secID] = instrument
	}
	records := make([]marketDataRecord, 0, len(history))
	rejected := 0
	var collectErr error
	loadedAt := time.Now().UTC()
	traded := make(map[string]bool, len(history))
	for _, data := range history {
		if err := ctx.Err(); err != nil {
			return records, rejected, errors.Join(collectErr, err)
		}
		if data.NumTrades != nil && *data.NumTrades == 0 {
			continue
		}
		secID := strings.TrimSpace(data.SECID)
		traded[secID] = true
		sourceInstrument, exists := instrumentsBySECID[secID]
		if !exists || duplicates[secID] {
			rejected++
			collectErr = errors.Join(collectErr, fmt.Errorf("%w: missing or ambiguous instrument %s on %s", ErrInvalidData, secID, board))
			continue
		}
		if data.BoardID != board || sourceInstrument.BoardID != board || data.TradeDate != tradeDate.Format("2006-01-02") {
			rejected++
			collectErr = errors.Join(collectErr, fmt.Errorf("%w: unexpected board or date for %s", ErrInvalidData, secID))
			continue
		}
		instrument, err := NormalizeInstrument(sourceInstrument)
		if err != nil {
			if skipRow(err) {
				continue
			}
			rejected++
			collectErr = errors.Join(collectErr, fmt.Errorf("normalize instrument %s: %w", secID, err))
			continue
		}
		marketData, err := NormalizeMarketData(data, instrument.ID, loadedAt)
		if err != nil {
			if skipRow(err) {
				continue
			}
			rejected++
			collectErr = errors.Join(collectErr, fmt.Errorf("normalize market data for %s: %w", secID, err))
			continue
		}
		if instrument.Currency != *marketData.PriceCurrency {
			rejected++
			collectErr = errors.Join(collectErr, fmt.Errorf("%w: inconsistent currencies for %s", ErrInvalidData, secID))
			continue
		}
		if (instrument.CouponType == "floating") != (strings.TrimSpace(data.BondType) == "Облигация с плавающим купоном") {
			rejected++
			collectErr = errors.Join(collectErr, fmt.Errorf("%w: inconsistent coupon type for %s", ErrInvalidData, secID))
			continue
		}
		records = append(records, marketDataRecord{instrument: instrument, marketData: &marketData, sourceInstrument: sourceInstrument, sourceMarketData: data})
	}
	for _, sourceInstrument := range instruments {
		if err := ctx.Err(); err != nil {
			return records, rejected, errors.Join(collectErr, err)
		}
		secID := strings.TrimSpace(sourceInstrument.SECID)
		if traded[secID] || duplicates[secID] {
			continue
		}
		if sourceInstrument.BoardID != board {
			collectErr = errors.Join(collectErr, fmt.Errorf("%w: unexpected reference board for %s", ErrInvalidData, secID))
			continue
		}
		instrument, err := NormalizeInstrument(sourceInstrument)
		if err != nil {
			if !skipRow(err) {
				collectErr = errors.Join(collectErr, fmt.Errorf("normalize instrument %s: %w", secID, err))
			}
			continue
		}
		records = append(records, marketDataRecord{instrument: instrument})
	}
	return records, rejected, collectErr
}

func skipRow(err error) bool {
	return errors.Is(err, ErrUnsupportedCurrency) || errors.Is(err, ErrUnsupportedInstrument) || errors.Is(err, ErrNoTrades)
}
