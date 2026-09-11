ALTER TABLE turnstile.outbox_events
    ADD COLUMN status text NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING', 'COMPLETED', 'FAILED')),
    ADD COLUMN attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    ADD COLUMN available_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN locked_until timestamptz,
    ADD COLUMN lock_token text,
    ADD COLUMN last_error text,
    ADD COLUMN completed_at timestamptz,
    ADD CONSTRAINT outbox_events_lock_consistency CHECK (
        (locked_until IS NULL AND lock_token IS NULL) OR
        (locked_until IS NOT NULL AND lock_token IS NOT NULL)
    ),
    ADD CONSTRAINT outbox_events_completion_consistency CHECK (
        (status = 'COMPLETED' AND completed_at IS NOT NULL) OR
        (status <> 'COMPLETED' AND completed_at IS NULL)
    );

ALTER TABLE turnstile.outbox_events
    DROP CONSTRAINT outbox_events_transaction_id_fkey,
    ADD CONSTRAINT outbox_events_transaction_id_fkey
        FOREIGN KEY (transaction_id) REFERENCES turnstile.transactions(id);

CREATE UNIQUE INDEX outbox_events_transaction_event_type_idx
ON turnstile.outbox_events (transaction_id, event_type);

CREATE INDEX outbox_events_pending_idx
ON turnstile.outbox_events (available_at, id)
WHERE status = 'PENDING';
