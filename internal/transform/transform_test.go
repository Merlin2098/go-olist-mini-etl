package transform

import (
	"sort"
	"testing"

	"go_proy1/internal/customer"
)

func sendCustomers(cs []customer.Customer) <-chan customer.Customer {
	ch := make(chan customer.Customer, len(cs))
	for _, c := range cs {
		ch <- c
	}
	close(ch)
	return ch
}

func drainCustomers(ch <-chan customer.Customer) []customer.Customer {
	var got []customer.Customer
	for c := range ch {
		got = append(got, c)
	}
	return got
}

func drainErrors(ch <-chan error) []error {
	var got []error
	for err := range ch {
		got = append(got, err)
	}
	return got
}

func TestNormalize(t *testing.T) {
	t.Run("title-cases city and passes valid rows through", func(t *testing.T) {
		in := sendCustomers([]customer.Customer{
			{ID: "1", UniqueID: "u1", City: "sao bernardo do campo", State: "SP"},
		})

		out, errs := Normalize(in)
		got := drainCustomers(out)
		gotErrs := drainErrors(errs)

		if len(gotErrs) != 0 {
			t.Fatalf("unexpected errors: %v", gotErrs)
		}
		if len(got) != 1 {
			t.Fatalf("got %d customers, want 1", len(got))
		}
		if want := "Sao Bernardo Do Campo"; got[0].City != want {
			t.Errorf("City = %q, want %q", got[0].City, want)
		}
	})

	t.Run("rejects rows with invalid state", func(t *testing.T) {
		in := sendCustomers([]customer.Customer{
			{ID: "1", UniqueID: "u1", City: "franca", State: "SaoPaulo"},
		})

		out, errs := Normalize(in)
		got := drainCustomers(out)
		gotErrs := drainErrors(errs)

		if len(got) != 0 {
			t.Fatalf("got %d customers, want 0 (invalid state should not pass through)", len(got))
		}
		if len(gotErrs) != 1 {
			t.Fatalf("got %d errors, want 1", len(gotErrs))
		}
	})
}

func TestAggregate(t *testing.T) {
	in := sendCustomers([]customer.Customer{
		{ID: "order-1", UniqueID: "u1", City: "Franca", State: "SP"},
		{ID: "order-2", UniqueID: "u1", City: "Franca", State: "SP"}, // same person, 2nd order
		{ID: "order-3", UniqueID: "u2", City: "Joinville", State: "SC"},
	})

	got := make([]CustomerSummary, 0)
	for s := range Aggregate(in) {
		got = append(got, s)
	}

	if len(got) != 2 {
		t.Fatalf("got %d summaries, want 2 unique customers", len(got))
	}

	sort.Slice(got, func(i, j int) bool { return got[i].UniqueID < got[j].UniqueID })

	if got[0].UniqueID != "u1" || got[0].OrderCount != 2 {
		t.Errorf("u1 summary = %+v, want OrderCount=2", got[0])
	}
	if got[1].UniqueID != "u2" || got[1].OrderCount != 1 {
		t.Errorf("u2 summary = %+v, want OrderCount=1", got[1])
	}
}
