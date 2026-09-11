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
	HTTP     HTTP
	Database Database
	LogLevel slog.Level
}

type HTTP struct {
	Address         string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

type Database struct {
	URL            string
	MaxConnections int32
	MinConnections int32
	ConnectTimeout time.Duration
	HealthTimeout  time.Duration
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

	cfg := Config{
		HTTP: HTTP{
			Address:         env("HTTP_ADDR", ":8080"),
			ReadTimeout:     readTimeout,
			WriteTimeout:    writeTimeout,
			IdleTimeout:     idleTimeout,
			ShutdownTimeout: shutdownTimeout,
		},
		Database: Database{
			URL:            strings.TrimSpace(os.Getenv("DATABASE_URL")),
			MaxConnections: maxConnections,
			MinConnections: minConnections,
			ConnectTimeout: connectTimeout,
			HealthTimeout:  healthTimeout,
		},
	}

	if cfg.Database.URL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.Database.MaxConnections < 1 || cfg.Database.MinConnections < 0 || cfg.Database.MinConnections > cfg.Database.MaxConnections {
		return Config{}, errors.New("database connection limits are invalid")
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
