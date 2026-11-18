package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iriski-app/backend/internal/clients/cbr"
	"github.com/iriski-app/backend/internal/storage/models"
	"github.com/iriski-app/backend/internal/storage/postgres"
)

func main() {
	if err := run(); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		slog.Error("FX collector failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	flags := flag.NewFlagSet("fx-collector", flag.ContinueOnError)
	dateValue := flags.String("date", "", "Date in YYYY-MM-DD format")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if *dateValue == "" || flags.NArg() != 0 {
		return errors.New("--date is required and positional arguments are not supported")
	}
	date, err := time.Parse("2006-01-02", *dateValue)
	if err != nil || date.IsZero() {
		return errors.New("invalid --date, expected YYYY-MM-DD")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result, err := cbr.NewClient(os.Getenv("CBR_BASE_URL"), 30*time.Second).GetRates(ctx, date)
	if err != nil {
		return err
	}
	db, err := postgres.CreateDB()
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get database connection: %w", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			slog.Error("close database", "error", err)
		}
	}()
	if err := postgres.CreateTables(db); err != nil {
		return err
	}
	loadedAt := time.Now().UTC()
	rates := make([]models.FXRate, 0, len(result.Items))
	for _, rate := range result.Items {
		value := rate.Value.FloatString(12)
		storedValue, ok := new(big.Rat).SetString(value)
		if !ok || storedValue.Cmp(rate.Value) != 0 {
			return fmt.Errorf("rate for %s exceeds the supported precision", rate.Currency)
		}
		rates = append(rates, models.FXRate{Currency: rate.Currency, EffectiveDate: result.EffectiveDate, Source: "cbr", Nominal: rate.Nominal, Value: value, LoadedAt: loadedAt})
	}
	if err := postgres.NewStore(db).SaveFXRates(ctx, result.RequestedDate, rates); err != nil {
		return fmt.Errorf("save FX rates: %w", err)
	}
	slog.Info("FX rates saved", "requested_date", result.RequestedDate.Format("2006-01-02"), "effective_date", result.EffectiveDate.Format("2006-01-02"), "count", len(rates))
	return nil
}
