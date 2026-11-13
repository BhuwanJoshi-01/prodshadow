-- Adding a nullable column is metadata-only and takes milliseconds, but it
-- needs ACCESS EXCLUSIVE on invoices. Behind the open report transaction the
-- request waits, and every API query on invoices waits behind the request.
ALTER TABLE invoices ADD COLUMN external_reference text;
