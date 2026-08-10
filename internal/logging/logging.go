// Package logging drains error channels produced by pipeline stages.
// It is intentionally decoupled from extract/transform/load: any stage
// that emits <-chan error can be wired into it without knowing how
// errors are ultimately reported.
package logging

import (
	"log"
	"sync"
)

// Errors consumes errs until it is closed, logging each one.
// It returns immediately; call it in its own goroutine.
func Errors(wg *sync.WaitGroup, errs <-chan error) {
	defer wg.Done()
	for err := range errs {
		log.Printf("pipeline error: %v", err)
	}
}
