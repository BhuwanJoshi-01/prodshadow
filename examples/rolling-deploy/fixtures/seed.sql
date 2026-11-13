INSERT INTO products (id, name, legacy_sku, sku, price)
SELECT g, 'Product ' || g, 'L-' || g, 'SKU-' || g, (g % 300) + 0.5
FROM generate_series(1, 20000) AS g;
