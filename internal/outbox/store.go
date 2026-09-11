package outbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Event is one durably stored message claimed for delivery.
type Event struct {
	ID            int64
	Type          string
	TransactionID int64
	Payload       json.RawMessage
	AttemptCount  int
	LockToken     string
}

// Database provides operations needed for outbox state transitions.
type Database interface {
	Begin(context.Context) (pgx.Tx, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// Store owns durable outbox state transitions.
type Store struct {
	db Database
}

// NewStore creates an outbox store.
func NewStore(db Database) *Store {
	return &Store{db: db}
}

// Claim leases up to limit ready events without waiting for other claimers.
func (s *Store) Claim(ctx context.Context, limit int, lease time.Duration) ([]Event, error) {
	if limit < 1 || lease <= 0 {
		return nil, errors.New("outbox claim limit and lease must be positive")
	}
	token, err := claimToken()
	if err != nil {
		return nil, err
	}

	var events []Event
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			WITH candidates AS (
				SELECT id
				FROM turnstile.outbox_events
				WHERE status = 'PENDING'
					AND available_at <= now()
					AND (locked_until IS NULL OR locked_until <= now())
				ORDER BY available_at, id
				LIMIT $1
				FOR UPDATE SKIP LOCKED
			)
			UPDATE turnstile.outbox_events AS event
			SET locked_until = now() + make_interval(secs => $2),
				lock_token = $3
			FROM candidates
			WHERE event.id = candidates.id
			RETURNING event.id, event.event_type, event.transaction_id, event.payload,
				event.attempt_count, event.lock_token`, limit, lease.Seconds(), token)
		if err != nil {
			return fmt.Errorf("claim outbox events: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var event Event
			if err := rows.Scan(&event.ID, &event.Type, &event.TransactionID, &event.Payload, &event.AttemptCount, &event.LockToken); err != nil {
				return fmt.Errorf("scan claimed outbox event: %w", err)
			}
			events = append(events, event)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("read claimed outbox events: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return events, nil
}

// Complete marks a claimed event delivered. A stale claim cannot change state.
func (s *Store) Complete(ctx context.Context, event Event) (bool, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE turnstile.outbox_events
		SET status = 'COMPLETED', completed_at = now(), locked_until = NULL,
			lock_token = NULL, last_error = NULL
		WHERE id = $1 AND status = 'PENDING' AND lock_token = $2`, event.ID, event.LockToken)
	if err != nil {
		return false, fmt.Errorf("complete outbox event: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// Fail records a delivery failure and either schedules retry or exposes terminal failure.
func (s *Store) Fail(ctx context.Context, event Event, cause error, retryAfter time.Duration, maxAttempts int) (bool, error) {
	if cause == nil || retryAfter <= 0 || maxAttempts < 1 {
		return false, errors.New("outbox failure cause, retry delay, and maximum attempts are required")
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE turnstile.outbox_events
		SET attempt_count = attempt_count + 1,
			status = CASE WHEN attempt_count + 1 >= $4 THEN 'FAILED' ELSE 'PENDING' END,
			available_at = now() + make_interval(secs => $3),
			locked_until = NULL,
			lock_token = NULL,
			last_error = left($2, 2000)
		WHERE id = $1 AND status = 'PENDING' AND lock_token = $5`,
		event.ID, cause.Error(), retryAfter.Seconds(), maxAttempts, event.LockToken)
	if err != nil {
		return false, fmt.Errorf("fail outbox event: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func claimToken() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate outbox claim token: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}
