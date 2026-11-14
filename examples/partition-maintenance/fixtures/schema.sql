-- Events are partitioned by year; the application reads and writes the
-- current year's partition through the parent table.
CREATE TABLE events (
    id         bigint NOT NULL,
    account_id bigint NOT NULL,
    kind       text NOT NULL,
    at         timestamptz NOT NULL,
    PRIMARY KEY (id, at)
) PARTITION BY RANGE (at);
CREATE TABLE events_2025 PARTITION OF events FOR VALUES FROM ('2025-01-01') TO ('2026-01-01');
CREATE TABLE events_2026 PARTITION OF events FOR VALUES FROM ('2026-01-01') TO ('2027-01-01');
CREATE INDEX events_account_idx ON events (account_id, at DESC);
