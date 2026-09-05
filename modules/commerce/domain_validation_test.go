package commerce

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateCustomDomain(t *testing.T) {
	tests := []struct {
		name           string
		domain         string
		platformDomain string
		wantErr        bool
	}{
		{
			name:           "valid custom domain",
			domain:         "shop.example.com",
			platformDomain: "matjero.com",
			wantErr:        false,
		},
		{
			name:           "valid custom domain with trailing dot",
			domain:         "shop.example.com.",
			platformDomain: "matjero.com",
			wantErr:        false,
		},
		{
			name:           "valid custom domain mixed case",
			domain:         "  Shop.Example.COM  ",
			platformDomain: "matjero.com",
			wantErr:        false,
		},
		{
			name:           "valid multi-level custom domain",
			domain:         "store.sub.brand.co.uk",
			platformDomain: "matjero.com",
			wantErr:        false,
		},
		{
			name:           "empty domain",
			domain:         "",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
		{
			name:           "whitespace only",
			domain:         "   ",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
		{
			name:           "scheme http",
			domain:         "http://shop.example.com",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
		{
			name:           "scheme https",
			domain:         "https://shop.example.com",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
		{
			name:           "port included",
			domain:         "shop.example.com:8080",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
		{
			name:           "path included",
			domain:         "shop.example.com/foo",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
		{
			name:           "credentials included",
			domain:         "user:pass@shop.example.com",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
		{
			name:           "wildcard domain",
			domain:         "*.example.com",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
		{
			name:           "IPv4 literal",
			domain:         "127.0.0.1",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
		{
			name:           "IPv6 literal",
			domain:         "::1",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
		{
			name:           "localhost",
			domain:         "localhost",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
		{
			name:           "single label host",
			domain:         "myhostname",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
		{
			name:           "label starts with hyphen",
			domain:         "-shop.example.com",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
		{
			name:           "label ends with hyphen",
			domain:         "shop-.example.com",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
		{
			name:           "underscore in label",
			domain:         "shop_store.example.com",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
		{
			name:           "non-ASCII characters",
			domain:         "shöp.example.com",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
		{
			name:           "label > 63 chars",
			domain:         strings.Repeat("a", 64) + ".example.com",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
		{
			name:           "claiming exact platform domain",
			domain:         "matjero.com",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
		{
			name:           "claiming subdomain of platform domain",
			domain:         "foo.matjero.com",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
		{
			name:           "claiming nested subdomain of platform domain",
			domain:         "bar.foo.matjero.com",
			platformDomain: "matjero.com",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCustomDomain(tt.domain, tt.platformDomain)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCustomDomain(%q, %q) error = %v, wantErr %v", tt.domain, tt.platformDomain, err, tt.wantErr)
			}
			if tt.wantErr && err != nil && !errors.Is(err, ErrInvalidInput) {
				t.Errorf("expected ErrInvalidInput, got %v", err)
			}
		})
	}
}
