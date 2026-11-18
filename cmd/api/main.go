package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iriski-app/backend/internal/api"
	"github.com/iriski-app/backend/internal/service"
	"github.com/iriski-app/backend/internal/storage/postgres"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("application failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
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
	svc := service.NewService(store)
	handler := api.NewHandler(svc)
	router := api.NewRouter(handler)
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8000"
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverErrors := make(chan error, 1)
	go func() {
		slog.Info("starting HTTP server", "addr", addr)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		stop()
	}

	slog.Info("shutting down HTTP server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		if closeErr := server.Close(); closeErr != nil {
			slog.Error("failed to close HTTP server", "error", closeErr)
		}
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}
	return nil
}
