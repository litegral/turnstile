CREATE TABLE turnstile.availability_destination (
    inventory_id bigint PRIMARY KEY,
    quantity bigint NOT NULL CHECK (quantity >= 0),
    version bigint NOT NULL CHECK (version > 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);
