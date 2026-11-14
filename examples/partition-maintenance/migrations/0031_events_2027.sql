-- Next year's partition, created the usual way. PARTITION OF needs ACCESS
-- EXCLUSIVE on the partitioned table: it waits for the export, and every
-- query on every partition queues behind it.
CREATE TABLE events_2027 PARTITION OF events
    FOR VALUES FROM ('2027-01-01') TO ('2028-01-01');
