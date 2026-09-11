package accounting

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/litegral/turnstile/internal/booking"
	"github.com/litegral/turnstile/internal/database"
	"github.com/litegral/turnstile/internal/outbox"
	"github.com/litegral/turnstile/migrations"
)

func TestBookingEventuallyReachesAccounting(t *testing.T) {
	pool := accountingIntegrationPool(t)
	if _, err := pool.Exec(context.Background(), `
		TRUNCATE turnstile.outbox_events, turnstile.booking_requests, turnstile.transactions, turnstile.ticket_inventory
		RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	var inventoryID int64
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO turnstile.ticket_inventory (ticket_type, available_quantity)
		VALUES ('VIP', 1) RETURNING id`).Scan(&inventoryID); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var keys []string
	requests := 0
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		requests++
		if requests < 3 {
			http.Error(w, "temporary", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer destination.Close()

	result, err := booking.NewService(pool).Book(context.Background(), booking.Request{
		InventoryID: inventoryID, CustomerID: "customer-1", Quantity: 1, IdempotencyKey: "accounting-e2e",
	})
	if err != nil {
		t.Fatal(err)
	}
	var transactions int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM turnstile.transactions WHERE id = $1`, result.TransactionID).Scan(&transactions); err != nil {
		t.Fatal(err)
	}
	if transactions != 1 {
		t.Fatalf("committed transactions = %d, want 1", transactions)
	}

	client := newTestClient(t, destination.URL, 5, time.Minute)
	worker, err := outbox.NewWorker(
		outbox.NewStoreForTypes(pool, EventType), client,
		outbox.Config{Concurrency: 1, PollInterval: 5 * time.Millisecond, LeaseDuration: time.Second, DeliveryTimeout: 500 * time.Millisecond, BaseRetryDelay: 10 * time.Millisecond, MaximumRetryDelay: 10 * time.Millisecond},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(ctx)
	}()

	var status string
	var attempts int
	for {
		err := pool.QueryRow(ctx, `
			SELECT status, attempt_count FROM turnstile.outbox_events
			WHERE transaction_id = $1 AND event_type = $2`, result.TransactionID, EventType).Scan(&status, &attempts)
		if err != nil {
			t.Fatal(err)
		}
		if status == "COMPLETED" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("accounting event status = %s after %d attempts", status, attempts)
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if attempts != 2 || len(keys) != 3 || keys[0] == "" || keys[0] != keys[1] || keys[1] != keys[2] {
		t.Fatalf("attempts = %d, idempotency keys = %v", attempts, keys)
	}
	var availabilityStatus string
	if err := pool.QueryRow(context.Background(), `
		SELECT status FROM turnstile.outbox_events
		WHERE transaction_id = $1 AND event_type = 'AVAILABILITY_CHANGED'`, result.TransactionID).Scan(&availabilityStatus); err != nil {
		t.Fatal(err)
	}
	if availabilityStatus != "PENDING" {
		t.Fatalf("availability status = %s, want PENDING", availabilityStatus)
	}
	t.Logf("accounting evidence: http_requests=%d failed_requests=%d successful_requests=1 recorded_attempts=%d status=%s stable_idempotency_key=%t committed_transactions=%d",
		len(keys), len(keys)-1, attempts, status, keys[0] == keys[1] && keys[1] == keys[2], transactions)
}

func accountingIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatal(err)
	}
	return pool
}
