package outbox

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/litegral/turnstile/internal/database"
	"github.com/litegral/turnstile/migrations"
)

func TestStoreConcurrentClaimAndCrashRecovery(t *testing.T) {
	pool := outboxIntegrationPool(t)
	resetOutboxTables(t, pool)
	firstID, secondID := insertOutboxFixture(t, pool)
	store := NewStore(pool)
	ctx := context.Background()

	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback(ctx) }()
	if _, err := blocker.Exec(ctx, `SELECT id FROM turnstile.outbox_events WHERE id = $1 FOR UPDATE`, firstID); err != nil {
		t.Fatal(err)
	}

	claimed, err := store.Claim(ctx, 2, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].ID != secondID {
		t.Fatalf("Claim() IDs = %v, want only %d", eventIDs(claimed), secondID)
	}
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	updated, err := store.Fail(ctx, claimed[0], errors.New("destination unavailable"), time.Second, 1)
	if err != nil || !updated {
		t.Fatalf("Fail() = %v, %v; want true, nil", updated, err)
	}
	var status, lastError string
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT status, attempt_count, last_error FROM turnstile.outbox_events WHERE id = $1`, secondID).Scan(&status, &attempts, &lastError); err != nil {
		t.Fatal(err)
	}
	if status != "FAILED" || attempts != 1 || lastError != "destination unavailable" {
		t.Fatalf("failed event = %q, %d, %q", status, attempts, lastError)
	}

	firstClaim, err := store.Claim(ctx, 1, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstClaim) != 1 || firstClaim[0].ID != firstID {
		t.Fatalf("first Claim() IDs = %v, want %d", eventIDs(firstClaim), firstID)
	}
	immediate, err := store.Claim(ctx, 1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(immediate) != 0 {
		t.Fatalf("immediate Claim() IDs = %v, want none", eventIDs(immediate))
	}

	time.Sleep(75 * time.Millisecond)
	reclaimed, err := store.Claim(ctx, 1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(reclaimed) != 1 || reclaimed[0].ID != firstID || reclaimed[0].LockToken == firstClaim[0].LockToken {
		t.Fatalf("reclaimed event = %+v, first claim = %+v", reclaimed, firstClaim)
	}
	updated, err = store.Complete(ctx, firstClaim[0])
	if err != nil || updated {
		t.Fatalf("stale Complete() = %v, %v; want false, nil", updated, err)
	}
	updated, err = store.Complete(ctx, reclaimed[0])
	if err != nil || !updated {
		t.Fatalf("current Complete() = %v, %v; want true, nil", updated, err)
	}
}

func outboxIntegrationPool(t *testing.T) *pgxpool.Pool {
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

func resetOutboxTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		TRUNCATE turnstile.outbox_events, turnstile.booking_requests, turnstile.transactions, turnstile.ticket_inventory
		RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("reset outbox tables: %v", err)
	}
}

func insertOutboxFixture(t *testing.T, pool *pgxpool.Pool) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	var inventoryID, transactionID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO turnstile.ticket_inventory (ticket_type, available_quantity)
		VALUES ('VIP', 1) RETURNING id`).Scan(&inventoryID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO turnstile.transactions
			(inventory_id, customer_id, quantity, idempotency_key, available_quantity, inventory_version)
		VALUES ($1, 'customer', 1, 'outbox-test', 0, 1) RETURNING id`, inventoryID).Scan(&transactionID); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, `
		WITH inserted AS (
			INSERT INTO turnstile.outbox_events (event_type, transaction_id, payload)
			VALUES ('ACCOUNTING_TRANSACTION_CREATED', $1, '{}'), ('AVAILABILITY_CHANGED', $1, '{}')
			RETURNING id
		)
		SELECT id FROM inserted ORDER BY id`, transactionID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	ids := make([]int64, 0, 2)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("inserted event count = %d, want 2", len(ids))
	}
	return ids[0], ids[1]
}

func eventIDs(events []Event) []int64 {
	ids := make([]int64, len(events))
	for i, event := range events {
		ids[i] = event.ID
	}
	return ids
}
