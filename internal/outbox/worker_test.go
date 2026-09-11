package outbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestNewWorkerValidation(t *testing.T) {
	valid := Config{
		Concurrency:       1,
		PollInterval:      time.Second,
		LeaseDuration:     2 * time.Second,
		DeliveryTimeout:   time.Second,
		BaseRetryDelay:    time.Second,
		MaximumRetryDelay: time.Minute,
		MaxAttempts:       3,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tests := []struct {
		name   string
		change func(*Config)
	}{
		{name: "zero concurrency", change: func(c *Config) { c.Concurrency = 0 }},
		{name: "zero attempts", change: func(c *Config) { c.MaxAttempts = 0 }},
		{name: "delivery exceeds lease", change: func(c *Config) { c.DeliveryTimeout = c.LeaseDuration }},
		{name: "base exceeds maximum", change: func(c *Config) { c.BaseRetryDelay = 2 * c.MaximumRetryDelay }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := valid
			tt.change(&config)
			_, err := NewWorker(&Store{}, HandlerFunc(func(context.Context, Event) error { return nil }), config, logger)
			if err == nil {
				t.Fatal("NewWorker() error = nil, want validation error")
			}
		})
	}
}

func TestWorkerProcessOne(t *testing.T) {
	tests := []struct {
		name         string
		handlerError error
		wantComplete int
		wantFail     int
	}{
		{name: "successful delivery", wantComplete: 1},
		{name: "failed delivery", handlerError: errors.New("temporary failure"), wantFail: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &stubStore{events: []Event{{ID: 7, Type: "TEST", LockToken: "claim"}}}
			worker, err := NewWorker(store, HandlerFunc(func(context.Context, Event) error {
				return tt.handlerError
			}), validWorkerConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			processed, err := worker.processOne(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !processed || store.completed != tt.wantComplete || store.failed != tt.wantFail {
				t.Fatalf("processed = %v, completed = %d, failed = %d", processed, store.completed, store.failed)
			}
		})
	}
}

func TestWorkerCancellationLeavesLeaseForRecovery(t *testing.T) {
	store := &stubStore{events: []Event{{ID: 7, LockToken: "claim"}}}
	worker, err := NewWorker(store, HandlerFunc(func(ctx context.Context, _ Event) error {
		<-ctx.Done()
		return ctx.Err()
	}), validWorkerConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = worker.processOne(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("processOne() error = %v, want context canceled", err)
	}
	if store.completed != 0 || store.failed != 0 {
		t.Fatalf("completed = %d, failed = %d; canceled claim must remain leased", store.completed, store.failed)
	}
}

func TestRetryDelay(t *testing.T) {
	for _, tt := range []struct {
		attempt int
		min     time.Duration
		max     time.Duration
	}{
		{attempt: 1, min: 500 * time.Millisecond, max: time.Second},
		{attempt: 2, min: time.Second, max: 2 * time.Second},
		{attempt: 10, min: 4 * time.Second, max: 8 * time.Second},
	} {
		t.Run(fmt.Sprintf("attempt %d", tt.attempt), func(t *testing.T) {
			delay, err := retryDelay(tt.attempt, time.Second, 8*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if delay < tt.min || delay > tt.max {
				t.Fatalf("retryDelay() = %v, want range [%v, %v]", delay, tt.min, tt.max)
			}
		})
	}
}

func validWorkerConfig() Config {
	return Config{
		Concurrency:       1,
		PollInterval:      time.Millisecond,
		LeaseDuration:     time.Second,
		DeliveryTimeout:   500 * time.Millisecond,
		BaseRetryDelay:    time.Second,
		MaximumRetryDelay: time.Minute,
		MaxAttempts:       3,
	}
}

type stubStore struct {
	events    []Event
	completed int
	failed    int
}

func (s *stubStore) Claim(context.Context, int, time.Duration) ([]Event, error) {
	return s.events, nil
}

func (s *stubStore) Complete(context.Context, Event) (bool, error) {
	s.completed++
	return true, nil
}

func (s *stubStore) Fail(context.Context, Event, error, time.Duration, int) (bool, error) {
	s.failed++
	return true, nil
}
