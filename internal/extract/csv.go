// Package extract reads the Olist customers CSV and streams validated
// Customer records over a channel, one goroutine per Extract call.
package extract

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"

	"go_proy1/internal/customer"
)

// expected column order in olist_customers_dataset.csv
const (
	colCustomerID = iota
	colUniqueID
	colZipCodePrefix
	colCity
	colState
)

// Extract opens path and streams each row as a Customer on the returned
// channel. Rows that fail to parse or fail Customer.Validate are sent on
// the error channel instead, and do not stop the read.
//
// Both channels are closed once the file has been fully read.
func Extract(path string) (<-chan customer.Customer, <-chan error) {
	customers := make(chan customer.Customer, 100)
	errs := make(chan error, 100)

	go func() {
		defer close(customers)
		defer close(errs)

		f, err := os.Open(path)
		if err != nil {
			errs <- fmt.Errorf("extract: open %s: %w", path, err)
			return
		}
		defer f.Close()

		r := csv.NewReader(f)

		if _, err := r.Read(); err != nil { // skip header
			errs <- fmt.Errorf("extract: read header: %w", err)
			return
		}

		for {
			record, err := r.Read()
			if err == io.EOF {
				return
			}
			if err != nil {
				errs <- fmt.Errorf("extract: read row: %w", err)
				continue
			}

			c := customer.Customer{
				ID:            record[colCustomerID],
				UniqueID:      record[colUniqueID],
				ZipCodePrefix: record[colZipCodePrefix],
				City:          record[colCity],
				State:         record[colState],
			}

			if err := c.Validate(); err != nil {
				errs <- fmt.Errorf("extract: invalid row (customer_id=%s): %w", c.ID, err)
				continue
			}

			customers <- c
		}
	}()

	return customers, errs
}
