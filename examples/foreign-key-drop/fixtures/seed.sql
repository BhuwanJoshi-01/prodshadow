INSERT INTO customers (id, email, country)
SELECT g, 'customer' || g || '@example.com', (ARRAY['DE', 'FR', 'IN', 'US'])[1 + g % 4]
FROM generate_series(1, 50000) AS g;

INSERT INTO invoices (id, customer_id, amount, issued_at)
SELECT g, 1 + (g::bigint * 7919) % 50000, (g % 500) + 0.99, timestamptz '2026-01-01' + g * interval '1 minute'
FROM generate_series(1, 300000) AS g;
