package load

import (
	"fmt"
	"io"
	"testing"

	"go_proy1/internal/transform"
)

// benchN mirrors the real pipeline's scale: ~96k unique customers.
const benchN = 96096

func summariesChan(n int) <-chan transform.CustomerSummary {
	ch := make(chan transform.CustomerSummary, 100)
	go func() {
		defer close(ch)
		for i := 0; i < n; i++ {
			ch <- transform.CustomerSummary{
				UniqueID:   fmt.Sprintf("customer-%d", i),
				City:       "Sao Paulo",
				State:      "SP",
				OrderCount: (i % 5) + 1,
			}
		}
	}()
	return ch
}

// BenchmarkWriteJSONArray measures the original strategy: drain the
// channel into a slice, then encode the whole slice at once.
func BenchmarkWriteJSONArray(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := writeJSONArray(io.Discard, summariesChan(benchN)); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWriteJSONLines measures the current strategy: encode each
// record as it arrives from the channel, no intermediate slice.
func BenchmarkWriteJSONLines(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := writeJSONLines(io.Discard, summariesChan(benchN)); err != nil {
			b.Fatal(err)
		}
	}
}
