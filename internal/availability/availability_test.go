package availability

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/litegral/turnstile/internal/outbox"
)

func TestHandlerRejectsInvalidEvents(t *testing.T) {
	tests := []struct {
		name  string
		event outbox.Event
	}{
		{name: "wrong type", event: outbox.Event{Type: "OTHER", Payload: json.RawMessage(`{}`)}},
		{name: "malformed payload", event: outbox.Event{Type: EventType, Payload: json.RawMessage(`{`)}},
		{name: "invalid values", event: outbox.Event{Type: EventType, Payload: json.RawMessage(`{"inventory_id":1,"quantity":-1,"version":2}`)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := &stubDatabase{}
			handler, err := NewHandler(db)
			if err != nil {
				t.Fatal(err)
			}
			err = handler.Handle(context.Background(), test.event)
			if !outbox.IsPermanent(err) {
				t.Fatalf("Handle() error = %v, want permanent error", err)
			}
			if db.called {
				t.Fatal("database called for invalid event")
			}
		})
	}
}

func TestHandlerRetriesDatabaseFailure(t *testing.T) {
	db := &stubDatabase{err: errors.New("database unavailable")}
	handler, err := NewHandler(db)
	if err != nil {
		t.Fatal(err)
	}
	err = handler.Handle(context.Background(), outbox.Event{
		Type:    EventType,
		Payload: json.RawMessage(`{"inventory_id":1,"quantity":2,"version":12}`),
	})
	if err == nil || outbox.IsPermanent(err) {
		t.Fatalf("Handle() error = %v, want temporary error", err)
	}
}

type stubDatabase struct {
	called bool
	err    error
}

func (d *stubDatabase) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	d.called = true
	return pgconn.CommandTag{}, d.err
}
