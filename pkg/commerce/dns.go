package commerce

import (
	"context"
	"fmt"
	"net"
)

// TXTRecordPrefix is the standard DNS TXT record label prefix for ownership verification.
const TXTRecordPrefix = "_matjero-verification"

// TXTValuePrefix is the standard prefix for the verification token value in TXT records.
const TXTValuePrefix = "matjero-verification"

// StoreDomainVerification describes the exact DNS TXT challenge instructions
// for a custom domain.
type StoreDomainVerification struct {
	RecordType  string `json:"record_type"`
	RecordName  string `json:"record_name"`
	RecordValue string `json:"record_value"`
}

// FormatTXTRecordName returns the authoritative DNS TXT record hostname for ownership verification.
func FormatTXTRecordName(domain string) string {
	normalized, err := NormalizeDomain(domain)
	if err != nil {
		normalized = domain
	}
	return fmt.Sprintf("%s.%s", TXTRecordPrefix, normalized)
}

// FormatTXTRecordValue returns the expected TXT record payload value for a verification token.
func FormatTXTRecordValue(token string) string {
	return fmt.Sprintf("%s=%s", TXTValuePrefix, token)
}

// BuildVerificationDetails constructs the DNS instructions payload for a verification token.
func BuildVerificationDetails(domain, token string) StoreDomainVerification {
	return StoreDomainVerification{
		RecordType:  "TXT",
		RecordName:  FormatTXTRecordName(domain),
		RecordValue: FormatTXTRecordValue(token),
	}
}

// TXTResolver abstracts DNS TXT record lookups for verification and testing.
type TXTResolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// DefaultTXTResolver delegates to net.Resolver (or net.DefaultResolver).
type DefaultTXTResolver struct {
	Resolver *net.Resolver
}

func (r DefaultTXTResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	res := r.Resolver
	if res == nil {
		res = net.DefaultResolver
	}
	return res.LookupTXT(ctx, name)
}

// FakeTXTResolver is a mock resolver for unit and integration testing.
type FakeTXTResolver struct {
	Records map[string][]string
	Err     error
}

func (f FakeTXTResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	records, ok := f.Records[name]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	}
	return records, nil
}
