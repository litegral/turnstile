// Package availability synchronizes versioned ticket availability projections.
package availability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/litegral/turnstile/internal/outbox"
)

const EventType = "AVAILABILITY_CHANGED"

type executor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// Handler applies availability events to the destination projection.
type Handler struct {
	db executor
}

// NewHandler creates an availability event handler.
func NewHandler(db executor) (*Handler, error) {
	if db == nil {
		return nil, errors.New("availability database is required")
	}
	return &Handler{db: db}, nil
}

// Handle stores an event only when its version is newer than destination state.
func (h *Handler) Handle(ctx context.Context, event outbox.Event) error {
	if event.Type != EventType {
		return outbox.Permanent(fmt.Errorf("unsupported availability event type %q", event.Type))
	}
	var payload struct {
		InventoryID int64 `json:"inventory_id"`
		Quantity    int64 `json:"quantity"`
		Version     int64 `json:"version"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return outbox.Permanent(fmt.Errorf("decode availability event: %w", err))
	}
	if payload.InventoryID <= 0 || payload.Quantity < 0 || payload.Version <= 0 {
		return outbox.Permanent(errors.New("availability event has invalid inventory_id, quantity, or version"))
	}
	if _, err := h.db.Exec(ctx, `
		INSERT INTO turnstile.availability_destination (inventory_id, quantity, version)
		VALUES ($1, $2, $3)
		ON CONFLICT (inventory_id) DO UPDATE
		SET quantity = EXCLUDED.quantity,
			version = EXCLUDED.version,
			updated_at = now()
		WHERE EXCLUDED.version > turnstile.availability_destination.version`,
		payload.InventoryID, payload.Quantity, payload.Version,
	); err != nil {
		return fmt.Errorf("apply availability event: %w", err)
	}
	return nil
}
