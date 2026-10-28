package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/clients/moex"
	"github.com/iriski-app/backend/internal/storage/models"
)

type Client interface {
	GetInstruments(ctx context.Context, board string) ([]moex.Instrument, error)
	GetMarketData(ctx context.Context, board string, date time.Time) ([]moex.MarketData, error)
}

type Storage interface {
	CreateETLRun(ctx context.Context, source models.Source, tradeDate time.Time) (models.ETLRun, error)
	UpsertInstrument(ctx context.Context, instrument models.Instrument) (models.Instrument, error)
	SaveMarketData(ctx context.Context, instrument models.Instrument, marketData models.MarketData) error
	FinishETLRun(ctx context.Context, id uuid.UUID, status models.ETLStatus, rowsLoaded int, rowsRejected int, errorMessage string) error
}

type Service struct {
	Store  Storage
	Client Client
}

type Result struct {
	ID           uuid.UUID
	Status       models.ETLStatus
	RowsLoaded   int
	RowsRejected int
}

type marketDataRecord struct {
	instrument models.Instrument
	marketData *models.MarketData
}

func NewService(store Storage, client Client) *Service {
	return &Service{
		Store:  store,
		Client: client,
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
	for _, board := range []string{"TQOB", "TQCB"} {
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
	for _, record := range records {
		if record.marketData != nil {
			counts[record.instrument.ISIN]++
		}
	}
	for _, record := range records {
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
		if err := s.Store.SaveMarketData(ctx, record.instrument, *record.marketData); err != nil {
			result.RowsRejected++
			runErr = errors.Join(runErr, fmt.Errorf("save market data for %s: %w", record.instrument.ISIN, err))
			break
		}
		result.RowsLoaded++
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
		secID := strings.TrimSpace(data.SECID)
		if data.NumTrades == nil || *data.NumTrades != 0 {
			traded[secID] = true
		}
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
		records = append(records, marketDataRecord{instrument: instrument, marketData: &marketData})
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
