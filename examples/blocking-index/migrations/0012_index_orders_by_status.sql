-- Builds the index while holding SHARE on orders: reads continue, but every
-- INSERT and UPDATE waits until the build completes.
CREATE INDEX orders_status_created_idx ON orders (status, created_at);
