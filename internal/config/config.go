package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTP       HTTP
	Database   Database
	Accounting Accounting
	Outbox     Outbox
	LogLevel   slog.Level
}

type HTTP struct {
	Address         string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	BookingTimeout  time.Duration
	WebhookTimeout  time.Duration
	ShutdownTimeout time.Duration
}

type Database struct {
	URL            string
	MaxConnections int32
	MinConnections int32
	ConnectTimeout time.Duration
	HealthTimeout  time.Duration
}

type Accounting struct {
	URL              string
	Timeout          time.Duration
	FailureThreshold int
	CircuitOpen      time.Duration
}

type Outbox struct {
	Concurrency       int
	PollInterval      time.Duration
	LeaseDuration     time.Duration
	DeliveryTimeout   time.Duration
	BaseRetryDelay    time.Duration
	MaximumRetryDelay time.Duration
}

func Load() (Config, error) {
	maxConnections, err := integer("DB_MAX_CONNECTIONS", 20)
	if err != nil {
		return Config{}, err
	}
	minConnections, err := integer("DB_MIN_CONNECTIONS", 2)
	if err != nil {
		return Config{}, err
	}
	readTimeout, err := duration("HTTP_READ_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	writeTimeout, err := duration("HTTP_WRITE_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	idleTimeout, err := duration("HTTP_IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		return Config{}, err
	}
	bookingTimeout, err := duration("BOOKING_TIMEOUT", 8*time.Second)
	if err != nil {
		return Config{}, err
	}
	webhookTimeout, err := duration("WEBHOOK_TIMEOUT", 8*time.Second)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := duration("SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	connectTimeout, err := duration("DB_CONNECT_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	healthTimeout, err := duration("DB_HEALTH_TIMEOUT", time.Second)
	if err != nil {
		return Config{}, err
	}
	accountingTimeout, err := duration("ACCOUNTING_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	circuitOpen, err := duration("ACCOUNTING_CIRCUIT_OPEN", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	pollInterval, err := duration("OUTBOX_POLL_INTERVAL", 250*time.Millisecond)
	if err != nil {
		return Config{}, err
	}
	leaseDuration, err := duration("OUTBOX_LEASE_DURATION", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	deliveryTimeout, err := duration("OUTBOX_DELIVERY_TIMEOUT", 6*time.Second)
	if err != nil {
		return Config{}, err
	}
	baseRetryDelay, err := duration("OUTBOX_BASE_RETRY_DELAY", time.Second)
	if err != nil {
		return Config{}, err
	}
	maximumRetryDelay, err := duration("OUTBOX_MAX_RETRY_DELAY", time.Minute)
	if err != nil {
		return Config{}, err
	}
	accountingFailureThreshold, err := integer("ACCOUNTING_FAILURE_THRESHOLD", 5)
	if err != nil {
		return Config{}, err
	}
	outboxConcurrency, err := integer("OUTBOX_CONCURRENCY", 4)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		HTTP: HTTP{
			Address:         env("HTTP_ADDR", ":8080"),
			ReadTimeout:     readTimeout,
			WriteTimeout:    writeTimeout,
			IdleTimeout:     idleTimeout,
			BookingTimeout:  bookingTimeout,
			WebhookTimeout:  webhookTimeout,
			ShutdownTimeout: shutdownTimeout,
		},
		Database: Database{
			URL:            strings.TrimSpace(os.Getenv("DATABASE_URL")),
			MaxConnections: maxConnections,
			MinConnections: minConnections,
			ConnectTimeout: connectTimeout,
			HealthTimeout:  healthTimeout,
		},
		Accounting: Accounting{
			URL:              env("ACCOUNTING_URL", "http://localhost:8081/transaction"),
			Timeout:          accountingTimeout,
			FailureThreshold: int(accountingFailureThreshold),
			CircuitOpen:      circuitOpen,
		},
		Outbox: Outbox{
			Concurrency:       int(outboxConcurrency),
			PollInterval:      pollInterval,
			LeaseDuration:     leaseDuration,
			DeliveryTimeout:   deliveryTimeout,
			BaseRetryDelay:    baseRetryDelay,
			MaximumRetryDelay: maximumRetryDelay,
		},
	}

	if cfg.Database.URL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.Database.MaxConnections < 1 || cfg.Database.MinConnections < 0 || cfg.Database.MinConnections > cfg.Database.MaxConnections {
		return Config{}, errors.New("database connection limits are invalid")
	}
	if cfg.HTTP.BookingTimeout >= cfg.HTTP.WriteTimeout || cfg.HTTP.WebhookTimeout >= cfg.HTTP.WriteTimeout {
		return Config{}, errors.New("BOOKING_TIMEOUT and WEBHOOK_TIMEOUT must be shorter than HTTP_WRITE_TIMEOUT")
	}
	if cfg.Accounting.Timeout >= cfg.Outbox.DeliveryTimeout || cfg.Outbox.DeliveryTimeout >= cfg.Outbox.LeaseDuration {
		return Config{}, errors.New("ACCOUNTING_TIMEOUT must be shorter than OUTBOX_DELIVERY_TIMEOUT, which must be shorter than OUTBOX_LEASE_DURATION")
	}
	if cfg.Accounting.FailureThreshold < 1 || cfg.Outbox.Concurrency < 1 {
		return Config{}, errors.New("accounting and outbox counts must be positive")
	}
	if cfg.Outbox.BaseRetryDelay > cfg.Outbox.MaximumRetryDelay {
		return Config{}, errors.New("OUTBOX_BASE_RETRY_DELAY must not exceed OUTBOX_MAX_RETRY_DELAY")
	}

	level := strings.ToLower(env("LOG_LEVEL", "info"))
	if err := cfg.LogLevel.UnmarshalText([]byte(level)); err != nil {
		return Config{}, fmt.Errorf("LOG_LEVEL: %w", err)
	}
	return cfg, nil
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func duration(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return parsed, nil
}

func integer(name string, fallback int32) (int32, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	return int32(parsed), nil
}
