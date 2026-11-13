CREATE TABLE subscribers (
    id               bigint PRIMARY KEY,
    email            text NOT NULL,
    email_normalized text,
    opted_in         boolean NOT NULL,
    updated_at       timestamptz NOT NULL
);
