package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/turnstile")
	t.Setenv("ACCOUNTING_URL", "http://accounting.test/transaction")

	tests := []struct {
		name    string
		key     string
		value   string
		wantErr bool
	}{
		{name: "defaults"},
		{name: "invalid pool limits", key: "DB_MIN_CONNECTIONS", value: "21", wantErr: true},
		{name: "invalid integer", key: "DB_MAX_CONNECTIONS", value: "many", wantErr: true},
		{name: "invalid duration", key: "HTTP_READ_TIMEOUT", value: "soon", wantErr: true},
		{name: "non-positive duration", key: "DB_HEALTH_TIMEOUT", value: "0s", wantErr: true},
		{name: "booking timeout exceeds write timeout", key: "BOOKING_TIMEOUT", value: "10s", wantErr: true},
		{name: "invalid log level", key: "LOG_LEVEL", value: "verbose", wantErr: true},
		{name: "accounting timeout exceeds delivery", key: "ACCOUNTING_TIMEOUT", value: "6s", wantErr: true},
		{name: "zero outbox concurrency", key: "OUTBOX_CONCURRENCY", value: "0", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.key != "" {
				t.Setenv(tt.key, tt.value)
			}
			_, err := Load()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
