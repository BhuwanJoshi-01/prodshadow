CREATE TABLE orders (
    id          bigint PRIMARY KEY,
    customer_id bigint NOT NULL,
    amount      numeric(12, 2) NOT NULL,
    created_at  timestamptz NOT NULL
);

-- Revenue per day and customer, read by the dashboard.
CREATE MATERIALIZED VIEW daily_revenue AS
SELECT date_trunc('day', created_at) AS day, customer_id, sum(amount) AS revenue, count(*) AS orders
FROM orders
GROUP BY 1, 2;

-- REFRESH ... CONCURRENTLY needs a unique index on the view.
CREATE UNIQUE INDEX daily_revenue_day_customer ON daily_revenue (day, customer_id);
CREATE INDEX daily_revenue_customer ON daily_revenue (customer_id, day);
