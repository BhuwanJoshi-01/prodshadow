INSERT INTO users (id, email, plan, created_at)
SELECT g, 'user' || g || '@example.test', (ARRAY['free', 'team', 'enterprise'])[1 + g % 3],
       timestamptz '2024-01-01' + g * interval '5 minutes'
FROM generate_series(1, 50000) AS g;
