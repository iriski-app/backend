package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iriski-app/backend/internal/clients/moex"
	"github.com/iriski-app/backend/internal/collector"
	"github.com/iriski-app/backend/internal/storage/postgres"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		slog.Error("collector failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	tradeDate, err := parseDate(os.Args[1:])
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
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
			slog.Error("failed to close database connection", "error", err)
		}
	}()
	if err := postgres.CreateTables(db); err != nil {
		return err
	}
	store := postgres.NewStore(db)
	client := moex.NewClient(os.Getenv("MOEX_BASE_URL"), 30*time.Second)
	service := collector.NewService(store, client)
	_, err = service.Run(ctx, tradeDate)
	return err
}

func parseDate(args []string) (time.Time, error) {
	flags := flag.NewFlagSet("collector", flag.ContinueOnError)
	date := flags.String("date", "", "Trade date in YYYY-MM-DD format")
	if err := flags.Parse(args); err != nil {
		return time.Time{}, err
	}
	if flags.NArg() != 0 {
		return time.Time{}, errors.New("unexpected positional arguments")
	}
	if *date == "" {
		return time.Time{}, errors.New("--date is required")
	}
	tradeDate, err := time.Parse("2006-01-02", *date)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --date, expected YYYY-MM-DD: %w", err)
	}
	if tradeDate.IsZero() || tradeDate.Year() < 1 {
		return time.Time{}, errors.New("invalid --date")
	}
	return tradeDate, nil
}
