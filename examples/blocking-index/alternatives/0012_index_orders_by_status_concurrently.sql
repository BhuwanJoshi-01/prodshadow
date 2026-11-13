-- The non-blocking variant. It must run outside a transaction block; with
-- migration.transaction: auto ProdShadow runs it in autocommit mode.
CREATE INDEX CONCURRENTLY orders_status_created_idx ON orders (status, created_at);
