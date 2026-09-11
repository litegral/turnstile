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

	"github.com/litegral/turnstile/internal/booking"
	"github.com/litegral/turnstile/internal/config"
)

type bookingStub struct {
	result  booking.Result
	err     error
	request booking.Request
}

func (s *bookingStub) Book(_ context.Context, request booking.Request) (booking.Result, error) {
	s.request = request
	return s.result, s.err
}

type waitingBookingStub struct{}

func (waitingBookingStub) Book(ctx context.Context, _ booking.Request) (booking.Result, error) {
	<-ctx.Done()
	return booking.Result{}, ctx.Err()
}

func TestBookTickets(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		key            string
		service        *bookingStub
		wantStatus     int
		wantCustomerID string
	}{
		{
			name:           "created",
			body:           `{"inventory_id":1,"customer_id":"customer-1","quantity":1}`,
			key:            "request-1",
			service:        &bookingStub{result: booking.Result{TransactionID: 7}},
			wantStatus:     http.StatusCreated,
			wantCustomerID: "customer-1",
		},
		{
			name:       "replayed",
			body:       `{"inventory_id":1,"customer_id":"customer-1","quantity":1}`,
			key:        "request-1",
			service:    &bookingStub{result: booking.Result{TransactionID: 7, Replay: true}},
			wantStatus: http.StatusOK,
		},
		{name: "missing idempotency key", body: `{}`, service: &bookingStub{}, wantStatus: http.StatusBadRequest},
		{name: "unknown field", body: `{"inventory_id":1,"extra":true}`, key: "request-1", service: &bookingStub{}, wantStatus: http.StatusBadRequest},
		{name: "sold out", body: `{"inventory_id":1,"customer_id":"customer-1","quantity":1}`, key: "request-1", service: &bookingStub{err: booking.ErrSoldOut}, wantStatus: http.StatusConflict},
		{name: "not found", body: `{"inventory_id":1,"customer_id":"customer-1","quantity":1}`, key: "request-1", service: &bookingStub{err: booking.ErrInventoryNotFound}, wantStatus: http.StatusNotFound},
		{name: "database failure", body: `{"inventory_id":1,"customer_id":"customer-1","quantity":1}`, key: "request-1", service: &bookingStub{err: errors.New("down")}, wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			server := NewServer(config.HTTP{}, time.Second, pinger{}, tt.service, nil, logger)
			request := httptest.NewRequest(http.MethodPost, "/bookings", strings.NewReader(tt.body))
			request.Header.Set("Idempotency-Key", tt.key)
			response := httptest.NewRecorder()

			server.Handler.ServeHTTP(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, tt.wantStatus, response.Body.String())
			}
			if tt.wantCustomerID != "" && tt.service.request.CustomerID != tt.wantCustomerID {
				t.Fatalf("customer ID = %q, want %q", tt.service.request.CustomerID, tt.wantCustomerID)
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not JSON: %v", err)
			}
		})
	}
}

func TestBookTicketsTimesOut(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := NewServer(config.HTTP{BookingTimeout: time.Millisecond}, time.Second, pinger{}, waitingBookingStub{}, nil, logger)
	request := httptest.NewRequest(http.MethodPost, "/bookings", strings.NewReader(`{"inventory_id":1,"customer_id":"customer-1","quantity":1}`))
	request.Header.Set("Idempotency-Key", "request-1")
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}
