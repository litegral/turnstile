package accounting

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/litegral/turnstile/internal/outbox"
)

const EventType = "ACCOUNTING_TRANSACTION_CREATED"

var ErrCircuitOpen = errors.New("accounting circuit breaker is open")

type Config struct {
	URL              string
	Timeout          time.Duration
	FailureThreshold int
	OpenDuration     time.Duration
}

type Client struct {
	url     string
	http    *http.Client
	breaker circuitBreaker
}

func NewClient(config Config) (*Client, error) {
	endpoint, err := url.ParseRequestURI(config.URL)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
		return nil, errors.New("accounting URL must be an absolute HTTP URL")
	}
	if config.Timeout <= 0 || config.FailureThreshold < 1 || config.OpenDuration <= 0 {
		return nil, errors.New("accounting timeout, failure threshold, and open duration must be positive")
	}
	return &Client{url: endpoint.String(), http: &http.Client{Timeout: config.Timeout}, breaker: circuitBreaker{threshold: config.FailureThreshold, openFor: config.OpenDuration}}, nil
}

func (c *Client) Handle(ctx context.Context, event outbox.Event) error {
	if event.Type != EventType {
		return outbox.Permanent(fmt.Errorf("unsupported accounting event type %q", event.Type))
	}
	if !c.breaker.allow(time.Now()) {
		return outbox.Postpone(ErrCircuitOpen)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(event.Payload))
	if err != nil {
		c.breaker.success()
		return outbox.Permanent(fmt.Errorf("build accounting request: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", strconv.FormatInt(event.ID, 10))
	response, err := c.http.Do(req)
	if err != nil {
		c.breaker.failure(time.Now())
		return fmt.Errorf("send accounting request: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 32<<10))
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		c.breaker.success()
		return nil
	}
	err = fmt.Errorf("accounting returned HTTP %d", response.StatusCode)
	if response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooEarly || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
		c.breaker.failure(time.Now())
		return err
	}
	c.breaker.success()
	return outbox.Permanent(err)
}

type circuitBreaker struct {
	mu        sync.Mutex
	threshold int
	openFor   time.Duration
	failures  int
	openedAt  time.Time
	probe     bool
}

func (b *circuitBreaker) allow(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failures < b.threshold {
		return true
	}
	if now.Sub(b.openedAt) < b.openFor || b.probe {
		return false
	}
	b.probe = true
	return true
}

func (b *circuitBreaker) success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.openedAt = time.Time{}
	b.probe = false
}

func (b *circuitBreaker) failure(now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.probe = false
	b.failures++
	if b.failures >= b.threshold {
		b.openedAt = now
	}
}
