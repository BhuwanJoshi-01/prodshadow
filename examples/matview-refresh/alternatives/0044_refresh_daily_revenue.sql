-- The same refresh without blocking readers: CONCURRENTLY builds the new
-- contents next to the old ones and applies the difference, holding only
-- EXCLUSIVE, which lets SELECTs on the view continue. It needs a unique
-- index on the view and takes longer.
REFRESH MATERIALIZED VIEW CONCURRENTLY daily_revenue;
