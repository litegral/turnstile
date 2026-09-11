package booking

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrInventoryNotFound   = errors.New("ticket inventory not found")
	ErrSoldOut             = errors.New("ticket inventory sold out")
	ErrIdempotencyConflict = errors.New("idempotency key already used for another request")
	ErrInvalidRequest      = errors.New("invalid booking request")
)

type Request struct {
	InventoryID    int64
	CustomerID     string
	Quantity       int64
	IdempotencyKey string
}

type Result struct {
	TransactionID     int64     `json:"transaction_id"`
	InventoryID       int64     `json:"inventory_id"`
	Quantity          int64     `json:"quantity"`
	AvailableQuantity int64     `json:"available_quantity"`
	InventoryVersion  int64     `json:"inventory_version"`
	CreatedAt         time.Time `json:"created_at"`
	Replay            bool      `json:"idempotent_replay"`
}

type Beginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

type Service struct {
	db Beginner
}

func NewService(db Beginner) *Service {
	return &Service{db: db}
}

func (s *Service) Book(ctx context.Context, request Request) (Result, error) {
	request.CustomerID = strings.TrimSpace(request.CustomerID)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.InventoryID <= 0 || request.Quantity <= 0 || request.CustomerID == "" || len(request.CustomerID) > 200 || request.IdempotencyKey == "" || len(request.IdempotencyKey) > 200 {
		return Result{}, ErrInvalidRequest
	}

	var result Result
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var claimed bool
		if err := tx.QueryRow(ctx, `
			WITH claim AS (
				INSERT INTO turnstile.booking_requests (idempotency_key, inventory_id, customer_id, quantity)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT DO NOTHING
				RETURNING 1
			)
			SELECT EXISTS (SELECT 1 FROM claim)`,
			request.IdempotencyKey, request.InventoryID, request.CustomerID, request.Quantity,
		).Scan(&claimed); err != nil {
			return fmt.Errorf("claim idempotency key: %w", err)
		}

		if !claimed {
			var existing Request
			var transactionID *int64
			if err := tx.QueryRow(ctx, `
				SELECT inventory_id, customer_id, quantity, transaction_id
				FROM turnstile.booking_requests
				WHERE idempotency_key = $1
				FOR UPDATE`, request.IdempotencyKey,
			).Scan(&existing.InventoryID, &existing.CustomerID, &existing.Quantity, &transactionID); err != nil {
				return fmt.Errorf("read idempotency key: %w", err)
			}
			if existing.InventoryID != request.InventoryID || existing.CustomerID != request.CustomerID || existing.Quantity != request.Quantity {
				return ErrIdempotencyConflict
			}
			if transactionID == nil {
				return errors.New("idempotency request has no transaction")
			}
			if err := scanResult(tx.QueryRow(ctx, `
				SELECT id, inventory_id, quantity, available_quantity, inventory_version, created_at
				FROM turnstile.transactions
				WHERE id = $1`, *transactionID), &result); err != nil {
				return fmt.Errorf("read idempotent transaction: %w", err)
			}
			result.Replay = true
			return nil
		}

		if err := tx.QueryRow(ctx, `
			UPDATE turnstile.ticket_inventory
			SET available_quantity = available_quantity - $2,
				version = version + 1,
				updated_at = now()
			WHERE id = $1 AND available_quantity >= $2
			RETURNING available_quantity, version`, request.InventoryID, request.Quantity,
		).Scan(&result.AvailableQuantity, &result.InventoryVersion); err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("reserve inventory: %w", err)
			}
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM turnstile.ticket_inventory WHERE id = $1)`, request.InventoryID).Scan(&exists); err != nil {
				return fmt.Errorf("check inventory: %w", err)
			}
			if !exists {
				return ErrInventoryNotFound
			}
			return ErrSoldOut
		}

		result.InventoryID = request.InventoryID
		result.Quantity = request.Quantity
		if err := tx.QueryRow(ctx, `
			INSERT INTO turnstile.transactions
				(inventory_id, customer_id, quantity, idempotency_key, available_quantity, inventory_version)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, created_at`,
			request.InventoryID, request.CustomerID, request.Quantity, request.IdempotencyKey,
			result.AvailableQuantity, result.InventoryVersion,
		).Scan(&result.TransactionID, &result.CreatedAt); err != nil {
			return fmt.Errorf("create transaction: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO turnstile.outbox_events (event_type, transaction_id, payload)
			VALUES
				('ACCOUNTING_TRANSACTION_CREATED', $1, jsonb_build_object(
					'transaction_id', $1::bigint, 'inventory_id', $2::bigint, 'customer_id', $3::text, 'quantity', $4::bigint
				)),
				('AVAILABILITY_CHANGED', $1, jsonb_build_object(
					'inventory_id', $2::bigint, 'quantity', $5::bigint, 'version', $6::bigint
				))`,
			result.TransactionID, request.InventoryID, request.CustomerID, request.Quantity,
			result.AvailableQuantity, result.InventoryVersion,
		); err != nil {
			return fmt.Errorf("create outbox events: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			UPDATE turnstile.booking_requests
			SET transaction_id = $2
			WHERE idempotency_key = $1`, request.IdempotencyKey, result.TransactionID,
		); err != nil {
			return fmt.Errorf("complete idempotency request: %w", err)
		}
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("book tickets: %w", err)
	}
	return result, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanResult(row rowScanner, result *Result) error {
	return row.Scan(
		&result.TransactionID,
		&result.InventoryID,
		&result.Quantity,
		&result.AvailableQuantity,
		&result.InventoryVersion,
		&result.CreatedAt,
	)
}
