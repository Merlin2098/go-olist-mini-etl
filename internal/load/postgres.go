package load

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver

	"go_proy1/internal/transform"
)

// batchSize caps how many rows accumulate before a transaction is
// committed. Large enough to amortize round-trip cost, small enough
// to bound memory and keep a single transaction from holding locks
// too long.
const batchSize = 1000

// OpenPostgres opens a connection pool. Callers own the returned *sql.DB
// and must Close it.
func OpenPostgres(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("load: open postgres: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("load: ping postgres: %w", err)
	}
	return db, nil
}

// Postgres consumes summaries and upserts them into customer_summary
// in batches of batchSize rows per transaction. Re-running the
// pipeline against the same rows is safe: ON CONFLICT updates the
// existing row instead of erroring or duplicating it.
//
// Unlike JSONLines, a batch failure rolls back only that batch — rows
// already committed in prior batches stay committed. This trades
// perfect all-or-nothing atomicity for bounded memory and the ability
// to resume without redoing already-committed work.
func Postgres(ctx context.Context, db *sql.DB, summaries <-chan transform.CustomerSummary) (int, error) {
	var total int
	batch := make([]transform.CustomerSummary, 0, batchSize)

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := upsertBatch(ctx, db, batch); err != nil {
			return err
		}
		total += len(batch)
		batch = batch[:0]
		return nil
	}

	for s := range summaries {
		batch = append(batch, s)
		if len(batch) == batchSize {
			if err := flush(); err != nil {
				return total, err
			}
		}
	}
	if err := flush(); err != nil {
		return total, err
	}

	return total, nil
}

func upsertBatch(ctx context.Context, db *sql.DB, batch []transform.CustomerSummary) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("load: begin tx: %w", err)
	}
	defer tx.Rollback() // no-op if Commit succeeds

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO customer_summary (customer_unique_id, customer_city, customer_state, order_count)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (customer_unique_id) DO UPDATE
		SET customer_city = EXCLUDED.customer_city,
		    customer_state = EXCLUDED.customer_state,
		    order_count = EXCLUDED.order_count
	`)
	if err != nil {
		return fmt.Errorf("load: prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, s := range batch {
		if _, err := stmt.ExecContext(ctx, s.UniqueID, s.City, s.State, s.OrderCount); err != nil {
			return fmt.Errorf("load: upsert %s: %w", s.UniqueID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("load: commit tx: %w", err)
	}
	return nil
}
