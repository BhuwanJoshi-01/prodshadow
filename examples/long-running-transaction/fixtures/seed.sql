INSERT INTO invoices (id, account_id, total_cents, state, issued_at)
SELECT g, 1 + g % 5000, (g * 37) % 100000, (ARRAY['draft', 'open', 'paid'])[1 + g % 3],
       timestamptz '2025-06-01' + g * interval '30 seconds'
FROM generate_series(1, 100000) AS g;
