CREATE TABLE products (
    id         bigint PRIMARY KEY,
    name       text NOT NULL,
    legacy_sku text,
    sku        text NOT NULL,
    price      numeric(10, 2) NOT NULL
);
