package commerce

import "strings"

// NormalizeDomain returns the canonical form of a storefront domain for storage
// and lookup. It trims surrounding whitespace, lowercases, strips an accidental
// trailing dot, and rejects empty or structurally invalid values.
//
// All persisted StoreDomain records MUST be canonicalized through this function
// so that domain lookups are case-insensitive and stable, and so the
// case-insensitive unique index (lower(domain)) cannot be bypassed by mixed-case
// or padded input. This is the single shared normalization routine used at both
// the write boundary (repository) and the read boundary (storefront resolver).
func NormalizeDomain(domain string) (string, error) {
	domain = strings.TrimSpace(domain)
	domain = strings.ToLower(domain)
	domain = strings.TrimSuffix(domain, ".")
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return "", ErrInvalidInput
	}
	// Stored domains are bare hostnames: reject ports, paths, and credentials.
	if strings.ContainsAny(domain, ":/@") {
		return "", ErrInvalidInput
	}
	return domain, nil
}
