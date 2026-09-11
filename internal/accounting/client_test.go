package accounting

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/litegral/turnstile/internal/outbox"
)

func TestClientDeliversWithStableIdempotencyKey(t *testing.T) {
	var mu sync.Mutex
	var keys []string
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if string(body) != `{"transaction_id":42}` || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %q", body)
		}
		mu.Lock()
		defer mu.Unlock()
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		attempts++
		if attempts < 3 {
			http.Error(w, "temporary", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, 5, time.Minute)
	store := &deliveryStore{event: outbox.Event{ID: 99, Type: EventType, Payload: []byte(`{"transaction_id":42}`)}}
	worker, err := outbox.NewWorker(store, client, outbox.Config{
		Concurrency: 1, PollInterval: time.Millisecond, LeaseDuration: time.Second,
		DeliveryTimeout: 500 * time.Millisecond, BaseRetryDelay: time.Millisecond,
		MaximumRetryDelay: time.Millisecond,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	store.cancel = cancel
	worker.Run(ctx)
	mu.Lock()
	defer mu.Unlock()
	if len(keys) != 3 || keys[0] != "99" || keys[1] != "99" || keys[2] != "99" {
		t.Fatalf("idempotency keys = %v", keys)
	}
	if store.failures != 2 || store.completions != 1 {
		t.Fatalf("failures = %d, completions = %d", store.failures, store.completions)
	}
}

func TestClientCircuitBreaker(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		http.Error(w, "temporary", http.StatusInternalServerError)
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, 2, time.Hour)
	event := outbox.Event{ID: 1, Type: EventType, Payload: []byte(`{}`)}
	for range 2 {
		if err := client.Handle(context.Background(), event); err == nil {
			t.Fatal("want failure")
		}
	}
	if err := client.Handle(context.Background(), event); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d", requests)
	}
}

func TestClientMarksPermanentResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "invalid", http.StatusBadRequest) }))
	defer server.Close()
	err := newTestClient(t, server.URL, 2, time.Minute).Handle(context.Background(), outbox.Event{ID: 1, Type: EventType, Payload: []byte(`{}`)})
	if !outbox.IsPermanent(err) {
		t.Fatalf("error = %v, want permanent", err)
	}
}

type deliveryStore struct {
	event       outbox.Event
	failures    int
	completions int
	completed   bool
	cancel      context.CancelFunc
}

func (s *deliveryStore) Claim(context.Context, int, time.Duration) ([]outbox.Event, error) {
	if s.completed {
		return nil, nil
	}
	return []outbox.Event{s.event}, nil
}

func (s *deliveryStore) Complete(context.Context, outbox.Event) (bool, error) {
	s.completions++
	s.completed = true
	s.cancel()
	return true, nil
}

func (s *deliveryStore) Fail(_ context.Context, _ outbox.Event, _ error, _ time.Duration, _ outbox.FailureDisposition) (bool, error) {
	s.failures++
	s.event.AttemptCount++
	return true, nil
}

func newTestClient(t *testing.T, endpoint string, threshold int, openFor time.Duration) *Client {
	t.Helper()
	client, err := NewClient(Config{URL: endpoint, Timeout: time.Second, FailureThreshold: threshold, OpenDuration: openFor})
	if err != nil {
		t.Fatal(err)
	}
	return client
}
