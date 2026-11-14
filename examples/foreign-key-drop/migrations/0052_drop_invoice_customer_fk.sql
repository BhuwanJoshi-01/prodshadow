-- Invoices move to a separate billing service; the constraint goes first.
-- The statement names invoices, but dropping a foreign key also removes
-- its triggers from customers, which needs ACCESS EXCLUSIVE there too.
ALTER TABLE invoices DROP CONSTRAINT invoices_customer_fk;
