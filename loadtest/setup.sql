\set ON_ERROR_STOP on

INSERT INTO turnstile.ticket_inventory (ticket_type, available_quantity)
VALUES ('load-' || :'run_id', CAST(:'request_count' AS bigint))
RETURNING id;
