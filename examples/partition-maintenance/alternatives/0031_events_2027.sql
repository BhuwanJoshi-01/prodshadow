-- The same partition without blocking: create the table on its own, give it
-- a CHECK constraint that matches the bounds so the attach can skip its
-- validation scan, then attach it, which only needs SHARE UPDATE EXCLUSIVE
-- on the partitioned table and does not conflict with readers or writers.
CREATE TABLE events_2027 (LIKE events INCLUDING DEFAULTS INCLUDING CONSTRAINTS);
ALTER TABLE events_2027 ADD CONSTRAINT events_2027_bounds
    CHECK (at >= '2027-01-01' AND at < '2028-01-01');
ALTER TABLE events ATTACH PARTITION events_2027
    FOR VALUES FROM ('2027-01-01') TO ('2028-01-01');
ALTER TABLE events_2027 DROP CONSTRAINT events_2027_bounds;
