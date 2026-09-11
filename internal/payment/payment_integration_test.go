package payment

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/litegral/turnstile/internal/booking"
	"github.com/litegral/turnstile/internal/database"
	"github.com/litegral/turnstile/migrations"
)

func TestConcurrentDuplicateWebhookCreatesOnePayment(t *testing.T) {
	pool := integrationPool(t)
	resetPaymentTables(t, pool)
	transactionID := insertTransaction(t, pool)
	service := NewService(pool)

	const attempts = 16
	start := make(chan struct{})
	results := make(chan Result, attempts)
	errs := make(chan error, attempts)
	var wait sync.WaitGroup
	for range attempts {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			result, err := service.Process(context.Background(), Request{
				Provider: "accounting", EventID: "event-1", PaymentID: "payment-1", TransactionID: transactionID,
			})
			results <- result
			errs <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("Process() error = %v", err)
		}
	}
	var webhookEventID, transactionPaymentID int64
	for result := range results {
		if webhookEventID == 0 {
			webhookEventID = result.WebhookEventID
			transactionPaymentID = result.TransactionPaymentID
		}
		if result.WebhookEventID != webhookEventID || result.TransactionPaymentID != transactionPaymentID {
			t.Fatalf("result IDs = %d, %d; want %d, %d", result.WebhookEventID, result.TransactionPaymentID, webhookEventID, transactionPaymentID)
		}
	}

	var events, payments int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM turnstile.webhook_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM turnstile.transaction_payments`).Scan(&payments); err != nil {
		t.Fatal(err)
	}
	if events != 1 || payments != 1 {
		t.Fatalf("webhook events = %d, payments = %d; want 1, 1", events, payments)
	}
}

func TestWebhookIdentifierConflict(t *testing.T) {
	pool := integrationPool(t)
	resetPaymentTables(t, pool)
	transactionID := insertTransaction(t, pool)
	service := NewService(pool)
	ctx := context.Background()

	if _, err := service.Process(ctx, Request{Provider: "accounting", EventID: "event-1", PaymentID: "payment-1", TransactionID: transactionID}); err != nil {
		t.Fatal(err)
	}
	_, err := service.Process(ctx, Request{Provider: "accounting", EventID: "event-1", PaymentID: "payment-2", TransactionID: transactionID})
	if !errors.Is(err, ErrIdentifierConflict) {
		t.Fatalf("Process() error = %v, want ErrIdentifierConflict", err)
	}
}

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("create test pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}
	if err := database.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return pool
}

func resetPaymentTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		TRUNCATE turnstile.transaction_payments, turnstile.webhook_events, turnstile.outbox_events,
		turnstile.booking_requests, turnstile.transactions, turnstile.ticket_inventory
		RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("reset payment tables: %v", err)
	}
}

func insertTransaction(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var inventoryID int64
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO turnstile.ticket_inventory (ticket_type, available_quantity)
		VALUES ('VIP', 1) RETURNING id`).Scan(&inventoryID); err != nil {
		t.Fatal(err)
	}
	result, err := booking.NewService(pool).Book(context.Background(), booking.Request{
		InventoryID: inventoryID, CustomerID: "customer-1", Quantity: 1, IdempotencyKey: fmt.Sprintf("payment-fixture-%d", inventoryID),
	})
	if err != nil {
		t.Fatal(err)
	}
	return result.TransactionID
}
