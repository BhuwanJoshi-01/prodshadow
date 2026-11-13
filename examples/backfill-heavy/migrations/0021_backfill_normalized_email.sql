-- One UPDATE over the whole table: every row stays locked until COMMIT, so
-- concurrent updates to any subscriber wait for the full backfill.
UPDATE subscribers SET email_normalized = lower(email);
