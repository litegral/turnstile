package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

func main() {
	failures, err := strconv.Atoi(env("MOCK_ACCOUNTING_FAILURES", "2"))
	if err != nil || failures < 0 {
		slog.Error("MOCK_ACCOUNTING_FAILURES must be a non-negative integer")
		os.Exit(1)
	}
	server := &mockServer{remainingFailures: failures, seen: make(map[string]struct{})}
	http.HandleFunc("POST /transaction", server.transaction)
	address := env("MOCK_ACCOUNTING_ADDR", ":8081")
	slog.Info("mock accounting listening", "address", address, "initial_failures", failures)
	if err := http.ListenAndServe(address, nil); err != nil {
		slog.Error("mock accounting stopped", "error", err)
		os.Exit(1)
	}
}

type mockServer struct {
	mu                sync.Mutex
	remainingFailures int
	seen              map[string]struct{}
}

func (s *mockServer) transaction(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		http.Error(w, "Idempotency-Key is required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.remainingFailures > 0 {
		s.remainingFailures--
		http.Error(w, "temporary accounting failure", http.StatusInternalServerError)
		return
	}
	if _, duplicate := s.seen[key]; duplicate {
		w.WriteHeader(http.StatusOK)
		return
	}
	s.seen[key] = struct{}{}
	w.WriteHeader(http.StatusCreated)
	_, _ = fmt.Fprintln(w, `{"status":"accepted"}`)
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
