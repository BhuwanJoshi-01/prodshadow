SET lock_timeout = '2s';
-- A nullable column without a volatile default is a catalog-only change.
ALTER TABLE users ADD COLUMN display_name text;
