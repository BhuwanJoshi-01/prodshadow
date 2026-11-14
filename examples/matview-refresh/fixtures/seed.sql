INSERT INTO orders (id, customer_id, amount, created_at)
SELECT g, 1 + (g::bigint * 7919) % 5000, (g % 900) + 0.5,
       timestamptz '2025-01-01' + (g % 365) * interval '1 day' + (g % 86400) * interval '1 second'
FROM generate_series(1, 1500000) AS g;

REFRESH MATERIALIZED VIEW daily_revenue;
