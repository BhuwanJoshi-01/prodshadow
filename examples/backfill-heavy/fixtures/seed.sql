INSERT INTO subscribers (id, email, opted_in, updated_at)
SELECT g, 'Subscriber.' || g || '@Example.Test', g % 4 <> 0, timestamptz '2025-03-01' + g * interval '1 second'
FROM generate_series(1, 400000) AS g;
