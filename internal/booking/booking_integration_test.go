package booking

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/litegral/turnstile/internal/database"
	"github.com/litegral/turnstile/migrations"
)

func TestConcurrentBookingDoesNotOversell(t *testing.T) {
	pool := integrationPool(t)
	resetBookingTables(t, pool)
	inventoryID := insertInventory(t, pool, 1)
	service := NewService(pool)

	const buyers = 32
	start := make(chan struct{})
	results := make(chan error, buyers)
	var wait sync.WaitGroup
	for i := range buyers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := service.Book(context.Background(), Request{
				InventoryID:    inventoryID,
				CustomerID:     fmt.Sprintf("customer-%d", i),
				Quantity:       1,
				IdempotencyKey: fmt.Sprintf("booking-%d", i),
			})
			results <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	var succeeded, soldOut int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrSoldOut):
			soldOut++
		default:
			t.Fatalf("unexpected booking error: %v", err)
		}
	}
	if succeeded != 1 || soldOut != buyers-1 {
		t.Fatalf("succeeded = %d, sold out = %d; want 1 and %d", succeeded, soldOut, buyers-1)
	}

	var available, transactions, events int64
	if err := pool.QueryRow(context.Background(), `SELECT available_quantity FROM turnstile.ticket_inventory WHERE id = $1`, inventoryID).Scan(&available); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM turnstile.transactions WHERE inventory_id = $1`, inventoryID).Scan(&transactions); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM turnstile.outbox_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if available != 0 || transactions != 1 || events != 2 {
		t.Fatalf("available = %d, transactions = %d, events = %d; want 0, 1, 2", available, transactions, events)
	}
}

func TestConcurrentIdempotentBooking(t *testing.T) {
	pool := integrationPool(t)
	resetBookingTables(t, pool)
	inventoryID := insertInventory(t, pool, 10)
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
			result, err := service.Book(context.Background(), Request{
				InventoryID:    inventoryID,
				CustomerID:     "customer-1",
				Quantity:       1,
				IdempotencyKey: "same-request",
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
			t.Fatalf("idempotent booking failed: %v", err)
		}
	}
	var transactionID int64
	for result := range results {
		if transactionID == 0 {
			transactionID = result.TransactionID
		}
		if result.TransactionID != transactionID {
			t.Fatalf("transaction ID = %d, want %d", result.TransactionID, transactionID)
		}
	}

	var available, transactions, events int64
	if err := pool.QueryRow(context.Background(), `SELECT available_quantity FROM turnstile.ticket_inventory WHERE id = $1`, inventoryID).Scan(&available); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM turnstile.transactions`).Scan(&transactions); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM turnstile.outbox_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if available != 9 || transactions != 1 || events != 2 {
		t.Fatalf("available = %d, transactions = %d, events = %d; want 9, 1, 2", available, transactions, events)
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

func resetBookingTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		TRUNCATE turnstile.outbox_events, turnstile.booking_requests, turnstile.transactions, turnstile.ticket_inventory
		RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("reset booking tables: %v", err)
	}
}

func insertInventory(t *testing.T, pool *pgxpool.Pool, quantity int64) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO turnstile.ticket_inventory (ticket_type, available_quantity)
		VALUES ('VIP', $1)
		RETURNING id`, quantity).Scan(&id); err != nil {
		t.Fatalf("insert inventory: %v", err)
	}
	return id
}
