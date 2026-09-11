package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/litegral/turnstile/internal/accounting"
	"github.com/litegral/turnstile/internal/booking"
	"github.com/litegral/turnstile/internal/config"
	"github.com/litegral/turnstile/internal/database"
	"github.com/litegral/turnstile/internal/httpapi"
	"github.com/litegral/turnstile/internal/outbox"
	"github.com/litegral/turnstile/internal/payment"
)

func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	pool, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer pool.Close()

	accountingClient, err := accounting.NewClient(accounting.Config{
		URL:              cfg.Accounting.URL,
		Timeout:          cfg.Accounting.Timeout,
		FailureThreshold: cfg.Accounting.FailureThreshold,
		OpenDuration:     cfg.Accounting.CircuitOpen,
	})
	if err != nil {
		return fmt.Errorf("create accounting client: %w", err)
	}
	worker, err := outbox.NewWorker(
		outbox.NewStoreForTypes(pool, accounting.EventType),
		accountingClient,
		outbox.Config{
			Concurrency:       cfg.Outbox.Concurrency,
			PollInterval:      cfg.Outbox.PollInterval,
			LeaseDuration:     cfg.Outbox.LeaseDuration,
			DeliveryTimeout:   cfg.Outbox.DeliveryTimeout,
			BaseRetryDelay:    cfg.Outbox.BaseRetryDelay,
			MaximumRetryDelay: cfg.Outbox.MaximumRetryDelay,
		},
		logger,
	)
	if err != nil {
		return fmt.Errorf("create accounting outbox worker: %w", err)
	}
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		worker.Run(workerCtx)
	}()
	defer func() {
		stopWorker()
		<-workerDone
	}()

	bookings := booking.NewService(pool)
	payments := payment.NewService(pool)
	server := httpapi.NewServer(cfg.HTTP, cfg.Database.HealthTimeout, pool, bookings, payments, logger)
	serveErr := make(chan error, 1)
	go func() {
		logger.Info("server listening", "address", cfg.HTTP.Address)
		serveErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
		if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP after shutdown: %w", err)
		}
		logger.Info("server stopped")
		return nil
	}
}
