package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/litegral/turnstile/internal/config"
)

type databasePinger interface {
	Ping(context.Context) error
}

func NewServer(cfg config.HTTP, healthTimeout time.Duration, db databasePinger, bookings bookingService, logger *slog.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		respond(w, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), healthTimeout)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			respond(w, http.StatusServiceUnavailable, "not ready")
			return
		}
		respond(w, http.StatusOK, "ready")
	})
	mux.HandleFunc("POST /bookings", bookTickets(bookings, cfg.BookingTimeout, logger))

	return &http.Server{
		Addr:         cfg.Address,
		Handler:      logRequests(logger, mux),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}
}

func respond(w http.ResponseWriter, status int, state string) {
	respondJSON(w, status, map[string]string{"status": state})
}

func respondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		logger.InfoContext(r.Context(), "http request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
	})
}
