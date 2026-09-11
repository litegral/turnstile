package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/litegral/turnstile/internal/booking"
)

const maxBookingBodyBytes = 4 << 10

type bookingService interface {
	Book(context.Context, booking.Request) (booking.Result, error)
}

func bookTickets(service bookingService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if idempotencyKey == "" {
			respondError(w, http.StatusBadRequest, "Idempotency-Key header is required")
			return
		}

		var body struct {
			InventoryID int64  `json:"inventory_id"`
			CustomerID  string `json:"customer_id"`
			Quantity    int64  `json:"quantity"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBookingBodyBytes))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			respondError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			respondError(w, http.StatusBadRequest, "body must contain one JSON object")
			return
		}

		result, err := service.Book(r.Context(), booking.Request{
			InventoryID:    body.InventoryID,
			CustomerID:     body.CustomerID,
			Quantity:       body.Quantity,
			IdempotencyKey: idempotencyKey,
		})
		switch {
		case err == nil:
			status := http.StatusCreated
			if result.Replay {
				status = http.StatusOK
			}
			respondJSON(w, status, result)
		case errors.Is(err, booking.ErrInvalidRequest):
			respondError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, booking.ErrInventoryNotFound):
			respondError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, booking.ErrSoldOut), errors.Is(err, booking.ErrIdempotencyConflict):
			respondError(w, http.StatusConflict, err.Error())
		default:
			logger.ErrorContext(r.Context(), "book tickets", "error", err)
			respondError(w, http.StatusInternalServerError, "internal server error")
		}
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}
