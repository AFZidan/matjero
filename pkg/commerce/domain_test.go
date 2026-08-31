package commerce

import "testing"

func TestNormalizeDomain(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"Shop.Example.com", "shop.example.com", false},
		{"  SHOP.EXAMPLE.COM  ", "shop.example.com", false},
		{"shop.example.com.", "shop.example.com", false},
		{"  MixedCase.COM  ", "mixedcase.com", false},
		{"", "", true},
		{"   ", "", true},
		{"shop.example.com:443", "", true},
		{"shop.example.com/path", "", true},
		{"user@shop.example.com", "", true},
	}
	for _, tc := range cases {
		got, err := NormalizeDomain(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("NormalizeDomain(%q) expected error, got %q", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizeDomain(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("NormalizeDomain(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
