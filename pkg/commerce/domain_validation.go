package commerce

import (
	"net"
	"strings"
)

// ValidateCustomDomain validates a seller-supplied custom domain name for storage
// and verification. It relies on NormalizeDomain for basic canonicalization and
// enforces strict DNS hostname rules for custom tenant domains.
func ValidateCustomDomain(domain string, platformDomain string) error {
	normalized, err := NormalizeDomain(domain)
	if err != nil {
		return err
	}

	// 1. Total hostname length must be <= 253 chars.
	if len(normalized) > 253 {
		return ErrInvalidInput
	}

	// 2. Reject scheme prefixes if raw input had any scheme (NormalizeDomain already rejects '/' and ':').
	lowerRaw := strings.ToLower(strings.TrimSpace(domain))
	if strings.Contains(lowerRaw, "://") {
		return ErrInvalidInput
	}

	// 3. Reject wildcards.
	if strings.Contains(normalized, "*") {
		return ErrInvalidInput
	}

	// 4. Reject IP literals (IPv4 and IPv6).
	if ip := net.ParseIP(normalized); ip != nil {
		return ErrInvalidInput
	}
	// Also check bracketed IPv6 if any got past normalization.
	if strings.HasPrefix(normalized, "[") || strings.HasSuffix(normalized, "]") {
		return ErrInvalidInput
	}

	// 5. Must contain at least one dot (reject localhost and single-label hostnames).
	if !strings.Contains(normalized, ".") {
		return ErrInvalidInput
	}

	// 6. Validate each label.
	labels := strings.Split(normalized, ".")
	if len(labels) < 2 {
		return ErrInvalidInput
	}

	for _, label := range labels {
		if len(label) < 1 || len(label) > 63 {
			return ErrInvalidInput
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return ErrInvalidInput
		}
		// Host labels must be ASCII alphanumeric or hyphen only (no underscores, no non-ASCII).
		for i := 0; i < len(label); i++ {
			c := label[i]
			isAlphaNum := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
			isHyphen := c == '-'
			if !isAlphaNum && !isHyphen {
				return ErrInvalidInput
			}
		}
	}

	// 7. Platform domain protection: reject custom domain if it claims a hostname
	// under the configured PlatformDomain (e.g. "foo.matjero.com" when PlatformDomain="matjero.com").
	if platformDomain != "" {
		normPlatform, err := NormalizeDomain(platformDomain)
		if err == nil && normPlatform != "" {
			if normalized == normPlatform || strings.HasSuffix(normalized, "."+normPlatform) {
				return ErrInvalidInput
			}
		}
	}

	return nil
}
