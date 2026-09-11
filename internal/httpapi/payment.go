package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/litegral/turnstile/internal/payment"
)

const maxPaymentWebhookBodyBytes = 4 << 10

type paymentService interface {
	Process(context.Context, payment.Request) (payment.Result, error)
}

func processPaymentWebhook(service paymentService, timeout time.Duration, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provider := strings.TrimSpace(r.PathValue("provider"))
		var body struct {
			EventID       string `json:"event_id"`
			PaymentID     string `json:"payment_id"`
			TransactionID int64  `json:"transaction_id"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPaymentWebhookBodyBytes))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			respondError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			respondError(w, http.StatusBadRequest, "body must contain one JSON object")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		result, err := service.Process(ctx, payment.Request{
			Provider:      provider,
			EventID:       body.EventID,
			PaymentID:     body.PaymentID,
			TransactionID: body.TransactionID,
		})
		switch {
		case err == nil:
			status := http.StatusCreated
			if result.Duplicate {
				status = http.StatusOK
			}
			respondJSON(w, status, result)
		case errors.Is(err, payment.ErrInvalidRequest):
			respondError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, payment.ErrTransactionNotFound):
			respondError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, payment.ErrIdentifierConflict):
			respondError(w, http.StatusConflict, err.Error())
		case errors.Is(err, context.DeadlineExceeded):
			respondError(w, http.StatusServiceUnavailable, "webhook processing timed out; retry the same event")
		case errors.Is(err, context.Canceled):
			return
		default:
			logger.ErrorContext(r.Context(), "process payment webhook", "provider", provider, "error", err)
			respondError(w, http.StatusInternalServerError, "internal server error")
		}
	}
}
