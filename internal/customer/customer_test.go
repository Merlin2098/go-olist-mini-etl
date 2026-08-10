package customer

import (
	"errors"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		c       Customer
		wantErr bool
	}{
		{
			name: "valid customer",
			c: Customer{
				ID:            "id-1",
				UniqueID:      "unique-1",
				ZipCodePrefix: "14409",
				City:          "franca",
				State:         "SP",
			},
			wantErr: false,
		},
		{
			name:    "missing customer_id",
			c:       Customer{UniqueID: "unique-1", State: "SP"},
			wantErr: true,
		},
		{
			name:    "missing customer_unique_id",
			c:       Customer{ID: "id-1", State: "SP"},
			wantErr: true,
		},
		{
			name:    "missing customer_state",
			c:       Customer{ID: "id-1", UniqueID: "unique-1"},
			wantErr: true,
		},
		{
			name:    "zero value customer",
			c:       Customer{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.c.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrMissingField) {
				t.Errorf("Validate() error = %v, want wrapped ErrMissingField", err)
			}
		})
	}
}
