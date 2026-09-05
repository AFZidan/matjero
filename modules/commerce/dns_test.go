package commerce

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestDNSFormatting(t *testing.T) {
	domain := "shop.example.com"
	token := "abc123xyz"

	recName := FormatTXTRecordName(domain)
	if recName != "_matjero-verification.shop.example.com" {
		t.Fatalf("FormatTXTRecordName(%q) = %q", domain, recName)
	}

	recVal := FormatTXTRecordValue(token)
	if recVal != "matjero-verification=abc123xyz" {
		t.Fatalf("FormatTXTRecordValue(%q) = %q", token, recVal)
	}

	details := BuildVerificationDetails(domain, token)
	if details.RecordType != "TXT" {
		t.Fatalf("RecordType = %q", details.RecordType)
	}
	if details.RecordName != "_matjero-verification.shop.example.com" {
		t.Fatalf("RecordName = %q", details.RecordName)
	}
	if details.RecordValue != "matjero-verification=abc123xyz" {
		t.Fatalf("RecordValue = %q", details.RecordValue)
	}
}

func TestFakeTXTResolver(t *testing.T) {
	resolver := FakeTXTResolver{
		Records: map[string][]string{
			"_matjero-verification.shop.example.com": {"matjero-verification=secret123"},
		},
	}

	ctx := context.Background()

	t.Run("found record", func(t *testing.T) {
		records, err := resolver.LookupTXT(ctx, "_matjero-verification.shop.example.com")
		if err != nil {
			t.Fatalf("LookupTXT: %v", err)
		}
		if len(records) != 1 || records[0] != "matjero-verification=secret123" {
			t.Fatalf("unexpected records: %v", records)
		}
	})

	t.Run("not found record", func(t *testing.T) {
		_, err := resolver.LookupTXT(ctx, "_matjero-verification.other.com")
		if err == nil {
			t.Fatal("expected error for missing host")
		}
		var dnsErr *net.DNSError
		if !errors.As(err, &dnsErr) || !dnsErr.IsNotFound {
			t.Fatalf("expected DNSError with IsNotFound, got %v", err)
		}
	})
}
