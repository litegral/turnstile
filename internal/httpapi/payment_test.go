package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/litegral/turnstile/internal/config"
	"github.com/litegral/turnstile/internal/payment"
)

type paymentStub struct {
	result  payment.Result
	err     error
	request payment.Request
}

func (s *paymentStub) Process(_ context.Context, request payment.Request) (payment.Result, error) {
	s.request = request
	return s.result, s.err
}

func TestProcessPaymentWebhook(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		service    *paymentStub
		wantStatus int
	}{
		{name: "created", body: `{"event_id":"event-1","payment_id":"payment-1","transaction_id":7}`, service: &paymentStub{result: payment.Result{TransactionID: 7}}, wantStatus: http.StatusCreated},
		{name: "duplicate", body: `{"event_id":"event-1","payment_id":"payment-1","transaction_id":7}`, service: &paymentStub{result: payment.Result{TransactionID: 7, Duplicate: true}}, wantStatus: http.StatusOK},
		{name: "unknown field", body: `{"event_id":"event-1","extra":true}`, service: &paymentStub{}, wantStatus: http.StatusBadRequest},
		{name: "invalid request", body: `{}`, service: &paymentStub{err: payment.ErrInvalidRequest}, wantStatus: http.StatusBadRequest},
		{name: "transaction missing", body: `{"event_id":"event-1","payment_id":"payment-1","transaction_id":7}`, service: &paymentStub{err: payment.ErrTransactionNotFound}, wantStatus: http.StatusNotFound},
		{name: "identifier conflict", body: `{"event_id":"event-1","payment_id":"payment-1","transaction_id":7}`, service: &paymentStub{err: payment.ErrIdentifierConflict}, wantStatus: http.StatusConflict},
		{name: "database failure", body: `{"event_id":"event-1","payment_id":"payment-1","transaction_id":7}`, service: &paymentStub{err: errors.New("down")}, wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			server := NewServer(config.HTTP{WebhookTimeout: time.Second}, time.Second, pinger{}, nil, tt.service, logger)
			request := httptest.NewRequest(http.MethodPost, "/webhooks/accounting/payments", strings.NewReader(tt.body))
			response := httptest.NewRecorder()
			server.Handler.ServeHTTP(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, tt.wantStatus, response.Body.String())
			}
			if tt.wantStatus == http.StatusCreated && tt.service.request.Provider != "accounting" {
				t.Fatalf("provider = %q, want accounting", tt.service.request.Provider)
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not JSON: %v", err)
			}
		})
	}
}
