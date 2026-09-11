CREATE TABLE turnstile.ticket_inventory (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ticket_type text NOT NULL CHECK (btrim(ticket_type) <> ''),
    available_quantity bigint NOT NULL CHECK (available_quantity >= 0),
    version bigint NOT NULL DEFAULT 0 CHECK (version >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE turnstile.transactions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    inventory_id bigint NOT NULL REFERENCES turnstile.ticket_inventory(id),
    customer_id text NOT NULL CHECK (btrim(customer_id) <> '' AND length(customer_id) <= 200),
    quantity bigint NOT NULL CHECK (quantity > 0),
    idempotency_key text NOT NULL UNIQUE CHECK (btrim(idempotency_key) <> '' AND length(idempotency_key) <= 200),
    available_quantity bigint NOT NULL CHECK (available_quantity >= 0),
    inventory_version bigint NOT NULL CHECK (inventory_version > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX transactions_inventory_id_idx ON turnstile.transactions(inventory_id);

CREATE TABLE turnstile.booking_requests (
    idempotency_key text PRIMARY KEY CHECK (btrim(idempotency_key) <> '' AND length(idempotency_key) <= 200),
    inventory_id bigint NOT NULL,
    customer_id text NOT NULL CHECK (btrim(customer_id) <> '' AND length(customer_id) <= 200),
    quantity bigint NOT NULL CHECK (quantity > 0),
    transaction_id bigint UNIQUE REFERENCES turnstile.transactions(id),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE turnstile.outbox_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_type text NOT NULL CHECK (event_type IN ('ACCOUNTING_TRANSACTION_CREATED', 'AVAILABILITY_CHANGED')),
    transaction_id bigint NOT NULL REFERENCES turnstile.transactions(id) ON DELETE CASCADE,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX outbox_events_transaction_id_idx ON turnstile.outbox_events(transaction_id);
