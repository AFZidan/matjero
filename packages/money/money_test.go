package money

import "testing"

func TestMoneyValidate(t *testing.T) {
	m, err := New(1250, "EGP")
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if m.AmountMinor != 1250 || m.Currency != "EGP" {
		t.Fatalf("unexpected money: %#v", m)
	}
}

func TestMoneyRejectsInvalidValues(t *testing.T) {
	tests := []Money{
		{AmountMinor: -1, Currency: "EGP"},
		{AmountMinor: 1, Currency: "egp"},
		{AmountMinor: 1, Currency: "EG"},
	}

	for _, tc := range tests {
		if err := tc.Validate(); err == nil {
			t.Fatalf("expected error for %#v", tc)
		}
	}
}

func TestMoneyArithmetic(t *testing.T) {
	left := MustNew(150, "SAR")
	right := MustNew(50, "SAR")

	sum, err := left.Add(right)
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if sum.AmountMinor != 200 {
		t.Fatalf("sum = %d", sum.AmountMinor)
	}

	diff, err := left.Sub(right)
	if err != nil {
		t.Fatalf("Sub returned error: %v", err)
	}
	if diff.AmountMinor != 100 {
		t.Fatalf("diff = %d", diff.AmountMinor)
	}

	discount, err := left.Multiply(15, 100)
	if err != nil {
		t.Fatalf("Multiply returned error: %v", err)
	}
	if discount.AmountMinor != 23 {
		t.Fatalf("discount = %d", discount.AmountMinor)
	}
}
