// Package load writes pipeline results to their final destination.
package load

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"go_proy1/internal/transform"
)

// JSONLines consumes summaries and writes one compact JSON object per
// line (JSON Lines / .jsonl) to path. Unlike a single JSON array, this
// lets Load stream records as they arrive from Aggregate instead of
// requiring the full result set in memory first, and each line stays
// independently parseable — the format S3/Athena/Glue-style pipelines
// expect.
func JSONLines(path string, summaries <-chan transform.CustomerSummary) (int, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, fmt.Errorf("load: create %s: %w", path, err)
	}
	defer f.Close()

	count, err := writeJSONLines(f, summaries)
	if err != nil {
		return count, err
	}
	return count, nil
}

// writeJSONLines streams summaries to w as compact JSON, one object
// per line, without buffering the full result set. Split out from
// JSONLines so it can be benchmarked against writeJSONArray without
// real file I/O in the way.
func writeJSONLines(w io.Writer, summaries <-chan transform.CustomerSummary) (int, error) {
	bw := bufio.NewWriter(w)
	defer bw.Flush()

	enc := json.NewEncoder(bw)

	var count int
	for s := range summaries {
		if err := enc.Encode(s); err != nil {
			return count, fmt.Errorf("load: encode json: %w", err)
		}
		count++
	}

	return count, nil
}

// writeJSONArray drains summaries into a slice first, then encodes the
// whole slice as a single indented JSON array. This is the pipeline's
// original Load strategy, kept here only as a benchmark baseline for
// writeJSONLines — see docs/benchmarks/streaming-vs-buffered-load.md.
func writeJSONArray(w io.Writer, summaries <-chan transform.CustomerSummary) (int, error) {
	var all []transform.CustomerSummary
	for s := range summaries {
		all = append(all, s)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(all); err != nil {
		return len(all), fmt.Errorf("load: encode json: %w", err)
	}

	return len(all), nil
}
