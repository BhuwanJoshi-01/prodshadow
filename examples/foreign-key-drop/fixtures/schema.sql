CREATE TABLE customers (
    id      bigint PRIMARY KEY,
    email   text NOT NULL,
    country text NOT NULL
);

CREATE TABLE invoices (
    id          bigint PRIMARY KEY,
    customer_id bigint NOT NULL CONSTRAINT invoices_customer_fk REFERENCES customers (id),
    amount      numeric(12, 2) NOT NULL,
    issued_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX invoices_customer_idx ON invoices (customer_id);
