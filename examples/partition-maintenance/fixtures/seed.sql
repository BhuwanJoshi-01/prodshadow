INSERT INTO events (id, account_id, kind, at)
SELECT g, 1 + g % 2000, (ARRAY['login', 'view', 'purchase'])[1 + g % 3],
       timestamptz '2025-07-01' + g * interval '2 minutes'
FROM generate_series(1, 200000) AS g;
