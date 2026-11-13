CREATE TABLE users (
    id         bigint PRIMARY KEY,
    email      text NOT NULL,
    plan       text NOT NULL,
    created_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX users_email_idx ON users (email);
