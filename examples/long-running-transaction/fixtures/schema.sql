CREATE TABLE invoices (
    id          bigint PRIMARY KEY,
    account_id  bigint NOT NULL,
    total_cents bigint NOT NULL,
    state       text NOT NULL,
    issued_at   timestamptz NOT NULL
);
CREATE INDEX invoices_account_idx ON invoices (account_id, issued_at DESC);
