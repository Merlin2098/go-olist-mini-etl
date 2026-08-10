// Package transform normalizes raw Customer rows and aggregates them
// into per-person summaries. The input CSV has one row per order, not
// per person: many rows can share the same UniqueID.
package transform

import (
	"fmt"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"go_proy1/internal/customer"
)

// titleCaser converts strings to title case following Portuguese
// capitalization rules, matching the language of the Olist dataset's
// city names.
var titleCaser = cases.Title(language.Portuguese)

// CustomerSummary is a derived, per-person view of the raw Customer
// rows: how many orders a given unique customer placed, and the
// (normalized) location associated with them.
type CustomerSummary struct {
	UniqueID   string `json:"customer_unique_id"`
	City       string `json:"customer_city"`
	State      string `json:"customer_state"`
	OrderCount int    `json:"order_count"`
}

// Normalize consumes raw Customer rows and emits one normalized
// Customer per input row (City title-cased, State validated). It is a
// pure streaming stage: one row in, one row (or one error) out.
func Normalize(in <-chan customer.Customer) (<-chan customer.Customer, <-chan error) {
	out := make(chan customer.Customer, 100)
	errs := make(chan error, 100)

	go func() {
		defer close(out)
		defer close(errs)

		for c := range in {
			if len(c.State) != 2 {
				errs <- fmt.Errorf("transform: invalid state %q for customer_id=%s", c.State, c.ID)
				continue
			}
			c.City = titleCaser.String(c.City)
			out <- c
		}
	}()

	return out, errs
}

// Aggregate consumes normalized Customer rows and groups them by
// UniqueID, counting orders per person. Unlike Normalize, this stage
// cannot emit anything until the input channel is fully drained: the
// order count for a given person isn't final until their last row has
// been seen.
func Aggregate(in <-chan customer.Customer) <-chan CustomerSummary {
	out := make(chan CustomerSummary, 100)

	go func() {
		defer close(out)

		seen := make(map[string]*CustomerSummary)
		for c := range in {
			s, ok := seen[c.UniqueID]
			if !ok {
				s = &CustomerSummary{
					UniqueID: c.UniqueID,
					City:     c.City,
					State:    c.State,
				}
				seen[c.UniqueID] = s
			}
			s.OrderCount++
		}

		for _, s := range seen {
			out <- *s
		}
	}()

	return out
}
