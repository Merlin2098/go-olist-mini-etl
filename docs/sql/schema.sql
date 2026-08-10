-- Run once against the target database before using load.Postgres.
-- The pipeline assumes this table already exists; it does not run
-- migrations itself.

CREATE TABLE IF NOT EXISTS customer_summary (
    customer_unique_id TEXT PRIMARY KEY,
    customer_city       TEXT NOT NULL,
    customer_state      CHAR(2) NOT NULL,
    order_count          INTEGER NOT NULL
);
