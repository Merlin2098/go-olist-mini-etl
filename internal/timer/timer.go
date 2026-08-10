// Package timer measures how long a pipeline stage takes to run.
// It is a cross-cutting concern like logging: stages don't know they
// are being timed, the caller decides to wrap them.
package timer

import (
	"log"
	"time"
)

// Track starts a stopwatch for the named stage and returns a function
// that stops it and logs the elapsed time. Intended for:
//
//	defer timer.Track("extract")()
func Track(stage string) func() {
	start := time.Now()
	return func() {
		log.Printf("[timer] %s took %s", stage, time.Since(start))
	}
}
