package availability

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/litegral/turnstile/internal/database"
	"github.com/litegral/turnstile/internal/outbox"
	"github.com/litegral/turnstile/migrations"
)

func TestOutOfOrderAvailabilityKeepsNewestVersion(t *testing.T) {
	pool := availabilityIntegrationPool(t)
	if _, err := pool.Exec(context.Background(), `TRUNCATE turnstile.availability_destination`); err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(pool)
	if err != nil {
		t.Fatal(err)
	}

	events := []outbox.Event{
		{Type: EventType, Payload: json.RawMessage(`{"inventory_id":1,"quantity":2,"version":12}`)},
		{Type: EventType, Payload: json.RawMessage(`{"inventory_id":1,"quantity":5,"version":11}`)},
	}
	for _, event := range events {
		if err := handler.Handle(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}

	var quantity, version int64
	if err := pool.QueryRow(context.Background(), `
		SELECT quantity, version FROM turnstile.availability_destination WHERE inventory_id = 1`,
	).Scan(&quantity, &version); err != nil {
		t.Fatal(err)
	}
	if quantity != 2 || version != 12 {
		t.Fatalf("quantity = %d, version = %d; want 2, 12", quantity, version)
	}
	t.Logf("availability evidence: delivered_versions=12,11 final_quantity=%d final_version=%d", quantity, version)
}

func TestConcurrentAvailabilityKeepsNewestVersion(t *testing.T) {
	pool := availabilityIntegrationPool(t)
	if _, err := pool.Exec(context.Background(), `TRUNCATE turnstile.availability_destination`); err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(pool)
	if err != nil {
		t.Fatal(err)
	}

	const versions = 32
	start := make(chan struct{})
	errs := make(chan error, versions)
	var wait sync.WaitGroup
	for version := int64(1); version <= versions; version++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			payload, err := json.Marshal(map[string]int64{"inventory_id": 1, "quantity": versions - version, "version": version})
			if err == nil {
				err = handler.Handle(context.Background(), outbox.Event{Type: EventType, Payload: payload})
			}
			errs <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	var quantity, version int64
	if err := pool.QueryRow(context.Background(), `
		SELECT quantity, version FROM turnstile.availability_destination WHERE inventory_id = 1`,
	).Scan(&quantity, &version); err != nil {
		t.Fatal(err)
	}
	if quantity != 0 || version != versions {
		t.Fatalf("quantity = %d, version = %d; want 0, %d", quantity, version, versions)
	}
}

func availabilityIntegrationPool(t *testing.T) *pgxpool.Pool {
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
