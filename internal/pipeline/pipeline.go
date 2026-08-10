// Package pipeline wires extract, transform and load into the single
// end-to-end run shared by every frontend (CLI flags, GUI) this
// project exposes. Keeping the orchestration here means a new
// frontend only has to collect inputs and call Run — it never
// reimplements the wiring between stages.
package pipeline

import (
	"context"
	"fmt"
	"sync"

	"go_proy1/internal/config"
	"go_proy1/internal/extract"
	"go_proy1/internal/load"
	"go_proy1/internal/transform"
)

// Target identifies where Run should send the aggregated summaries.
type Target string

const (
	TargetJSONLines Target = "jsonl"
	TargetPostgres  Target = "postgres"
)

// Options configures a single pipeline run.
type Options struct {
	// SourcePath is the CSV file to extract from.
	SourcePath string
	// Target selects the load destination.
	Target Target
	// JSONOutputPath is where summaries are written when Target is
	// TargetJSONLines. Ignored for TargetPostgres.
	JSONOutputPath string
}

// Result reports the outcome of a Run: how many unique customers were
// loaded, and every row-level error collected along the way. Row
// errors do not stop the run — only a Err at the load stage does.
type Result struct {
	Count  int
	Errors []error
}

// Run executes the full extract -> transform -> aggregate -> load
// pipeline described by opts and blocks until it completes.
//
// Row-level errors (a malformed CSV line, an invalid state code) are
// collected into Result.Errors rather than failing the run: this
// matches the existing CLI's behavior of logging and continuing. A
// non-nil error return means the load stage itself failed (bad DSN,
// unreachable database, file that can't be created) and no summaries
// were persisted.
func Run(ctx context.Context, opts Options) (Result, error) {
	rawCustomers, extractErrs := extract.Extract(opts.SourcePath)
	normalized, transformErrs := transform.Normalize(rawCustomers)
	summaries := transform.Aggregate(normalized)

	var errs []error
	var mu sync.Mutex
	collect := func(in <-chan error) {
		for err := range in {
			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); collect(extractErrs) }()
	go func() { defer wg.Done(); collect(transformErrs) }()

	var count int
	var loadErr error
	switch opts.Target {
	case TargetPostgres:
		count, loadErr = runPostgres(ctx, summaries)
	default:
		count, loadErr = load.JSONLines(opts.JSONOutputPath, summaries)
	}

	wg.Wait()

	if loadErr != nil {
		return Result{Count: count, Errors: errs}, loadErr
	}
	return Result{Count: count, Errors: errs}, nil
}

func runPostgres(ctx context.Context, summaries <-chan transform.CustomerSummary) (int, error) {
	cfg, err := config.LoadPostgres()
	if err != nil {
		return 0, err
	}

	db, err := load.OpenPostgres(cfg.DSN())
	if err != nil {
		return 0, err
	}
	defer db.Close()

	count, err := load.Postgres(ctx, db, summaries)
	if err != nil {
		return count, fmt.Errorf("pipeline: load postgres: %w", err)
	}
	return count, nil
}
