package outbox

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"time"
)

// Handler delivers one event. Implementations must use Event.ID as idempotency key.
type Handler interface {
	Handle(context.Context, Event) error
}

// HandlerFunc adapts a function into a Handler.
type HandlerFunc func(context.Context, Event) error

type permanentError struct{ error }
type postponedError struct{ error }

func (e permanentError) Unwrap() error { return e.error }
func (e postponedError) Unwrap() error { return e.error }

// Permanent marks a delivery error that must not be retried.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err}
}

// IsPermanent reports whether retrying a delivery cannot succeed unchanged.
func IsPermanent(err error) bool {
	var target permanentError
	return errors.As(err, &target)
}

// Postpone marks work rejected before a delivery attempt, such as by an open circuit.
func Postpone(err error) error {
	if err == nil {
		return nil
	}
	return postponedError{err}
}

func isPostponed(err error) bool {
	var target postponedError
	return errors.As(err, &target)
}

// Handle delivers one event.
func (f HandlerFunc) Handle(ctx context.Context, event Event) error {
	return f(ctx, event)
}

// Config controls worker concurrency, leasing, and retry scheduling.
type Config struct {
	Concurrency       int
	PollInterval      time.Duration
	LeaseDuration     time.Duration
	DeliveryTimeout   time.Duration
	BaseRetryDelay    time.Duration
	MaximumRetryDelay time.Duration
}

type FailureDisposition int

const (
	FailureTemporary FailureDisposition = iota
	FailurePermanent
	FailurePostponed
)

type eventStore interface {
	Claim(context.Context, int, time.Duration) ([]Event, error)
	Complete(context.Context, Event) (bool, error)
	Fail(context.Context, Event, error, time.Duration, FailureDisposition) (bool, error)
}

// Worker claims and delivers outbox events with at-least-once semantics.
type Worker struct {
	store   eventStore
	handler Handler
	config  Config
	logger  *slog.Logger
}

// NewWorker validates dependencies and creates a worker.
func NewWorker(store eventStore, handler Handler, config Config, logger *slog.Logger) (*Worker, error) {
	if store == nil || handler == nil || logger == nil {
		return nil, errors.New("outbox worker dependencies are required")
	}
	if config.Concurrency < 1 {
		return nil, errors.New("outbox worker concurrency must be positive")
	}
	if config.PollInterval <= 0 || config.LeaseDuration <= 0 || config.DeliveryTimeout <= 0 || config.BaseRetryDelay <= 0 || config.MaximumRetryDelay <= 0 {
		return nil, errors.New("outbox worker durations must be positive")
	}
	if config.DeliveryTimeout >= config.LeaseDuration {
		return nil, errors.New("outbox delivery timeout must be shorter than lease duration")
	}
	if config.BaseRetryDelay > config.MaximumRetryDelay {
		return nil, errors.New("outbox base retry delay must not exceed maximum retry delay")
	}
	return &Worker{store: store, handler: handler, config: config, logger: logger}, nil
}

// Run processes events until ctx is canceled.
func (w *Worker) Run(ctx context.Context) {
	var workers sync.WaitGroup
	workers.Add(w.config.Concurrency)
	for id := range w.config.Concurrency {
		go func() {
			defer workers.Done()
			w.runLoop(ctx, id)
		}()
	}
	workers.Wait()
}

func (w *Worker) runLoop(ctx context.Context, workerID int) {
	for {
		processed, err := w.processOne(ctx)
		if err != nil && ctx.Err() == nil {
			w.logger.Error("outbox worker iteration failed", "worker_id", workerID, "error", err)
		}
		if processed && err == nil {
			continue
		}
		if !wait(ctx, w.config.PollInterval) {
			return
		}
	}
}

func (w *Worker) processOne(ctx context.Context) (bool, error) {
	events, err := w.store.Claim(ctx, 1, w.config.LeaseDuration)
	if err != nil {
		return false, err
	}
	if len(events) == 0 {
		return false, nil
	}
	event := events[0]
	deliveryCtx, cancel := context.WithTimeout(ctx, w.config.DeliveryTimeout)
	err = w.handler.Handle(deliveryCtx, event)
	cancel()
	if err == nil {
		updated, completeErr := w.store.Complete(ctx, event)
		if completeErr != nil {
			return true, completeErr
		}
		if !updated {
			w.logger.Warn("outbox completion ignored for stale claim", "event_id", event.ID)
		}
		return true, nil
	}
	if ctx.Err() != nil {
		return true, ctx.Err()
	}

	attempt := event.AttemptCount + 1
	if isPostponed(err) {
		attempt = event.AttemptCount
	}
	delay, delayErr := retryDelay(attempt, w.config.BaseRetryDelay, w.config.MaximumRetryDelay)
	if delayErr != nil {
		return true, delayErr
	}
	disposition := FailureTemporary
	if IsPermanent(err) {
		disposition = FailurePermanent
	} else if isPostponed(err) {
		disposition = FailurePostponed
	}
	updated, failErr := w.store.Fail(ctx, event, err, delay, disposition)
	if failErr != nil {
		return true, failErr
	}
	if !updated {
		w.logger.Warn("outbox failure ignored for stale claim", "event_id", event.ID)
		return true, nil
	}
	w.logger.Warn("outbox delivery failed", "event_id", event.ID, "event_type", event.Type, "attempt", event.AttemptCount+1, "error", err)
	return true, nil
}

func retryDelay(attempt int, base, maximum time.Duration) (time.Duration, error) {
	delay := base
	for i := 1; i < attempt && delay < maximum; i++ {
		if delay > maximum/2 {
			delay = maximum
			break
		}
		delay *= 2
	}
	if delay > maximum {
		delay = maximum
	}
	half := delay / 2
	span := delay - half
	jitter, err := rand.Int(rand.Reader, big.NewInt(int64(span)+1))
	if err != nil {
		return 0, fmt.Errorf("generate outbox retry jitter: %w", err)
	}
	return half + time.Duration(jitter.Int64()), nil
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
