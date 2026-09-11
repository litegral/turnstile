package payment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrInvalidRequest      = errors.New("invalid payment webhook")
	ErrTransactionNotFound = errors.New("transaction not found")
	ErrIdentifierConflict  = errors.New("webhook identifier already used for different payment data")
)

type Request struct {
	Provider      string
	EventID       string
	PaymentID     string
	TransactionID int64
}

type Result struct {
	WebhookEventID       int64     `json:"webhook_event_id"`
	TransactionPaymentID int64     `json:"transaction_payment_id"`
	TransactionID        int64     `json:"transaction_id"`
	CreatedAt            time.Time `json:"created_at"`
	Duplicate            bool      `json:"duplicate"`
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

func (s *Service) Process(ctx context.Context, request Request) (Result, error) {
	request.Provider = strings.TrimSpace(request.Provider)
	request.EventID = strings.TrimSpace(request.EventID)
	request.PaymentID = strings.TrimSpace(request.PaymentID)
	if request.Provider == "" || len(request.Provider) > 100 || request.EventID == "" || len(request.EventID) > 200 || request.PaymentID == "" || len(request.PaymentID) > 200 || request.TransactionID <= 0 {
		return Result{}, ErrInvalidRequest
	}

	result := Result{TransactionID: request.TransactionID}
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var transactionExists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM turnstile.transactions WHERE id = $1)`, request.TransactionID).Scan(&transactionExists); err != nil {
			return fmt.Errorf("check transaction: %w", err)
		}
		if !transactionExists {
			return ErrTransactionNotFound
		}

		err := tx.QueryRow(ctx, `
			INSERT INTO turnstile.webhook_events (provider, event_id, payment_id, transaction_id)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (provider, event_id) DO NOTHING
			RETURNING id`, request.Provider, request.EventID, request.PaymentID, request.TransactionID,
		).Scan(&result.WebhookEventID)
		if errors.Is(err, pgx.ErrNoRows) {
			var existingPaymentID string
			var existingTransactionID int64
			if err := tx.QueryRow(ctx, `
				SELECT id, payment_id, transaction_id
				FROM turnstile.webhook_events
				WHERE provider = $1 AND event_id = $2`, request.Provider, request.EventID,
			).Scan(&result.WebhookEventID, &existingPaymentID, &existingTransactionID); err != nil {
				return fmt.Errorf("read webhook event: %w", err)
			}
			if existingPaymentID != request.PaymentID || existingTransactionID != request.TransactionID {
				return ErrIdentifierConflict
			}
			result.Duplicate = true
		} else if err != nil {
			return fmt.Errorf("create webhook event: %w", err)
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO turnstile.transaction_payments (provider, payment_id, transaction_id, webhook_event_id)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (provider, payment_id) DO NOTHING
			RETURNING id, created_at`, request.Provider, request.PaymentID, request.TransactionID, result.WebhookEventID,
		).Scan(&result.TransactionPaymentID, &result.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			var existingTransactionID int64
			if err := tx.QueryRow(ctx, `
				SELECT id, transaction_id, created_at
				FROM turnstile.transaction_payments
				WHERE provider = $1 AND payment_id = $2`, request.Provider, request.PaymentID,
			).Scan(&result.TransactionPaymentID, &existingTransactionID, &result.CreatedAt); err != nil {
				return fmt.Errorf("read transaction payment: %w", err)
			}
			if existingTransactionID != request.TransactionID {
				return ErrIdentifierConflict
			}
			result.Duplicate = true
			return nil
		}
		if err != nil {
			return fmt.Errorf("create transaction payment: %w", err)
		}
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("process payment webhook: %w", err)
	}
	return result, nil
}
