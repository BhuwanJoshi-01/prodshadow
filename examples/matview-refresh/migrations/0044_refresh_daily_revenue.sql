-- Recompute the view after correcting historical orders. A plain refresh
-- takes ACCESS EXCLUSIVE on the view for its whole duration.
REFRESH MATERIALIZED VIEW daily_revenue;
