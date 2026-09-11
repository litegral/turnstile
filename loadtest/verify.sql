\set ON_ERROR_STOP on

WITH facts AS (
    SELECT
        i.available_quantity,
        i.version,
        (SELECT count(*) FROM turnstile.transactions t
         WHERE t.inventory_id = i.id AND t.idempotency_key LIKE :'key_prefix' || '%') AS transaction_count,
        (SELECT COALESCE(sum(t.quantity), 0) FROM turnstile.transactions t
         WHERE t.inventory_id = i.id AND t.idempotency_key LIKE :'key_prefix' || '%') AS booked_quantity,
        (SELECT count(*) FROM turnstile.booking_requests br
         WHERE br.inventory_id = i.id AND br.idempotency_key LIKE :'key_prefix' || '%' AND br.transaction_id IS NOT NULL) AS completed_request_count,
        (SELECT count(*) FROM turnstile.outbox_events oe
         JOIN turnstile.transactions t ON t.id = oe.transaction_id
         WHERE t.inventory_id = i.id AND t.idempotency_key LIKE :'key_prefix' || '%') AS outbox_count,
        (SELECT count(*) FROM turnstile.outbox_events oe
         JOIN turnstile.transactions t ON t.id = oe.transaction_id
         WHERE t.inventory_id = i.id AND t.idempotency_key LIKE :'key_prefix' || '%'
           AND oe.event_type = 'ACCOUNTING_TRANSACTION_CREATED') AS accounting_event_count,
        (SELECT count(*) FROM turnstile.outbox_events oe
         JOIN turnstile.transactions t ON t.id = oe.transaction_id
         WHERE t.inventory_id = i.id AND t.idempotency_key LIKE :'key_prefix' || '%'
           AND oe.event_type = 'AVAILABILITY_CHANGED') AS availability_event_count
    FROM turnstile.ticket_inventory i
    WHERE i.id = CAST(:'inventory_id' AS bigint)
)
SELECT
    available_quantity,
    version,
    transaction_count,
    booked_quantity,
    completed_request_count,
    outbox_count,
    accounting_event_count,
    availability_event_count,
    available_quantity = 0
        AND version = CAST(:'expected' AS bigint)
        AND transaction_count = CAST(:'expected' AS bigint)
        AND transaction_count = CAST(:'http_successes' AS bigint)
        AND booked_quantity = CAST(:'expected' AS bigint)
        AND completed_request_count = CAST(:'expected' AS bigint)
        AND outbox_count = CAST(:'expected' AS bigint) * 2
        AND accounting_event_count = CAST(:'expected' AS bigint)
        AND availability_event_count = CAST(:'expected' AS bigint) AS passed
FROM facts;
