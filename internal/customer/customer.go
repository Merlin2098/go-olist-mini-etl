package customer

import (
	"errors"
	"fmt"
)

type Customer struct {
	ID            string
	UniqueID      string
	ZipCodePrefix string
	City          string
	State         string
}

var ErrMissingField = errors.New("missing required field")

func (c Customer) Validate() error {
	switch {
	case c.ID == "":
		return fmt.Errorf("customer_id: %w", ErrMissingField)
	case c.UniqueID == "":
		return fmt.Errorf("customer_unique_id: %w", ErrMissingField)
	case c.State == "":
		return fmt.Errorf("customer_state: %w", ErrMissingField)
	}
	return nil
}
