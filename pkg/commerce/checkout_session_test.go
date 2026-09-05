package commerce

import "testing"

func TestComputeFinalizeFingerprintIsDeterministicAndLineOrderIndependent(t *testing.T) {
	secondAddressLine := "Apartment 2"
	session := CheckoutSession{ID: "session-1", CustomerID: stringPtr("customer-1")}
	cart := Cart{ID: "cart-1", Items: []CartItem{
		{SellerListingID: "listing-b", SKUID: "sku-b", Quantity: 2, ExpectedUnitPriceMinor: 200, ExpectedCurrencyCode: "EGP"},
		{SellerListingID: "listing-a", SKUID: "sku-a", Quantity: 1, ExpectedUnitPriceMinor: 100, ExpectedCurrencyCode: "EGP"},
	}}
	request := FinalizeRequest{
		SessionID: "session-1",
		ShippingAddress: ShippingAddress{
			RecipientName: "Customer",
			AddressLine1:  "1 Main Street",
			AddressLine2:  &secondAddressLine,
			City:          "Cairo",
			CountryCode:   "EG",
		},
		ContactEmail: "customer@example.test",
	}

	first, err := ComputeFinalizeFingerprint(session, cart, request)
	if err != nil {
		t.Fatal(err)
	}
	cart.Items[0], cart.Items[1] = cart.Items[1], cart.Items[0]
	second, err := ComputeFinalizeFingerprint(session, cart, request)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("line ordering changed fingerprint: %s != %s", first, second)
	}

	request.ContactEmail = "different@example.test"
	changed, err := ComputeFinalizeFingerprint(session, cart, request)
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Fatal("semantic contact change did not change fingerprint")
	}
}

func TestComputeFinalizeFingerprintUsesAllCartLineAuthority(t *testing.T) {
	session := CheckoutSession{ID: "session-1"}
	request := FinalizeRequest{
		SessionID: "session-1",
		ShippingAddress: ShippingAddress{
			RecipientName: "Customer", AddressLine1: "1 Main", City: "Cairo", CountryCode: "EG",
		},
		ContactEmail: "customer@example.test",
	}
	baseCart := Cart{ID: "cart-1", Items: []CartItem{{
		SellerListingID: "listing-1", SKUID: "sku-1", Quantity: 1,
		ExpectedUnitPriceMinor: 100, ExpectedCurrencyCode: "EGP",
	}}}
	base, err := ComputeFinalizeFingerprint(session, baseCart, request)
	if err != nil {
		t.Fatal(err)
	}
	variants := []Cart{
		{ID: "cart-1", Items: []CartItem{{SellerListingID: "listing-2", SKUID: "sku-1", Quantity: 1, ExpectedUnitPriceMinor: 100, ExpectedCurrencyCode: "EGP"}}},
		{ID: "cart-1", Items: []CartItem{{SellerListingID: "listing-1", SKUID: "sku-2", Quantity: 1, ExpectedUnitPriceMinor: 100, ExpectedCurrencyCode: "EGP"}}},
		{ID: "cart-1", Items: []CartItem{{SellerListingID: "listing-1", SKUID: "sku-1", Quantity: 2, ExpectedUnitPriceMinor: 100, ExpectedCurrencyCode: "EGP"}}},
		{ID: "cart-1", Items: []CartItem{{SellerListingID: "listing-1", SKUID: "sku-1", Quantity: 1, ExpectedUnitPriceMinor: 101, ExpectedCurrencyCode: "EGP"}}},
		{ID: "cart-1", Items: []CartItem{{SellerListingID: "listing-1", SKUID: "sku-1", Quantity: 1, ExpectedUnitPriceMinor: 100, ExpectedCurrencyCode: "SAR"}}},
	}
	for i, variant := range variants {
		got, err := ComputeFinalizeFingerprint(session, variant, request)
		if err != nil {
			t.Fatalf("variant %d: %v", i, err)
		}
		if got == base {
			t.Fatalf("variant %d did not change fingerprint", i)
		}
	}
}

func TestComputeFinalizeFingerprintRejectsIncompleteSemanticInput(t *testing.T) {
	_, err := ComputeFinalizeFingerprint(
		CheckoutSession{ID: "session-1"},
		Cart{ID: "cart-1"},
		FinalizeRequest{SessionID: "session-1", ContactEmail: ""},
	)
	if err != ErrInvalidInput {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
}

func stringPtr(value string) *string { return &value }
