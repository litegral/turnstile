package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/litegral/turnstile/internal/config"
)

type pinger struct{ err error }

func (p pinger) Ping(context.Context) error { return p.err }

func TestHealthEndpoints(t *testing.T) {
	tests := []struct {
		name string
		path string
		db   pinger
		want int
	}{
		{name: "live", path: "/health", want: http.StatusOK},
		{name: "ready", path: "/ready", want: http.StatusOK},
		{name: "database unavailable", path: "/ready", db: pinger{err: errors.New("down")}, want: http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := NewServer(config.HTTP{}, time.Second, tt.db, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			response := httptest.NewRecorder()
			server.Handler.ServeHTTP(response, request)
			if response.Code != tt.want {
				t.Fatalf("status = %d, want %d", response.Code, tt.want)
			}
		})
	}
}
