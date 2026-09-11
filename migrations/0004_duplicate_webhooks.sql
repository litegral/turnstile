CREATE TABLE turnstile.webhook_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    provider text NOT NULL CHECK (btrim(provider) <> '' AND length(provider) <= 100),
    event_id text NOT NULL CHECK (btrim(event_id) <> '' AND length(event_id) <= 200),
    payment_id text NOT NULL CHECK (btrim(payment_id) <> '' AND length(payment_id) <= 200),
    transaction_id bigint NOT NULL REFERENCES turnstile.transactions(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, event_id)
);

CREATE TABLE turnstile.transaction_payments (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    provider text NOT NULL CHECK (btrim(provider) <> '' AND length(provider) <= 100),
    payment_id text NOT NULL CHECK (btrim(payment_id) <> '' AND length(payment_id) <= 200),
    transaction_id bigint NOT NULL REFERENCES turnstile.transactions(id),
    webhook_event_id bigint NOT NULL UNIQUE REFERENCES turnstile.webhook_events(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, payment_id)
);

CREATE INDEX transaction_payments_transaction_id_idx
ON turnstile.transaction_payments (transaction_id);
