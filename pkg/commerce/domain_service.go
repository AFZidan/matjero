package commerce

import (
	"context"
	"errors"
	"net"
	"time"
)

// ListStoreDomainsForSubject returns all domains for an authorized store.
func (s Service) ListStoreDomainsForSubject(ctx context.Context, subject, storeID string) ([]StoreDomain, error) {
	store, err := s.repo.GetStore(ctx, storeID)
	if err != nil {
		return nil, err
	}
	if _, err := s.RequireSellerAccess(ctx, subject, store.SellerID); err != nil {
		return nil, err
	}
	return s.repo.ListStoreDomains(ctx, storeID)
}

// RequestCustomStoreDomain validates and registers a seller-supplied custom domain in the PENDING state.
func (s Service) RequestCustomStoreDomain(ctx context.Context, subject, storeID, domain string) (StoreDomain, error) {
	store, err := s.repo.GetStore(ctx, storeID)
	if err != nil {
		return StoreDomain{}, err
	}
	if _, err := s.RequireSellerAccess(ctx, subject, store.SellerID); err != nil {
		return StoreDomain{}, err
	}
	if err := ValidateCustomDomain(domain, s.PlatformDomain); err != nil {
		return StoreDomain{}, err
	}
	return s.repo.CreateCustomStoreDomain(ctx, storeID, domain)
}

// VerifyCustomStoreDomainForSubject performs a bounded DNS lookup to verify TXT record ownership.
func (s Service) VerifyCustomStoreDomainForSubject(ctx context.Context, subject, storeID, domainID string) (StoreDomain, error) {
	store, err := s.repo.GetStore(ctx, storeID)
	if err != nil {
		return StoreDomain{}, err
	}
	if _, err := s.RequireSellerAccess(ctx, subject, store.SellerID); err != nil {
		return StoreDomain{}, err
	}

	domainObj, err := s.repo.GetStoreDomain(ctx, domainID)
	if err != nil {
		return StoreDomain{}, err
	}
	if domainObj.StoreID != storeID {
		return StoreDomain{}, ErrNotFound
	}
	if domainObj.DomainType != "custom" {
		return StoreDomain{}, ErrInvalidInput
	}
	// Re-checking active or already verified domain is idempotent.
	if domainObj.Status == "active" || domainObj.Status == "verified" {
		return domainObj, nil
	}
	if domainObj.VerificationToken == nil || *domainObj.VerificationToken == "" {
		return StoreDomain{}, ErrInvalidInput
	}

	// Bounded DNS lookup timeout (5 seconds).
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resolver := s.TXTResolver
	if resolver == nil {
		resolver = DefaultTXTResolver{}
	}

	recName := FormatTXTRecordName(domainObj.Domain)
	expectedVal := FormatTXTRecordValue(*domainObj.VerificationToken)

	records, err := resolver.LookupTXT(lookupCtx, recName)
	now := time.Now()

	if err != nil {
		var netErr net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary())) {
			return domainObj, ErrUnavailable
		}
		// Authoritative failure / NXDOMAIN -> status failed, last_checked_at updated.
		return s.repo.UpdateDomainVerificationStatus(ctx, domainID, "failed", nil, &now)
	}

	matched := false
	for _, rec := range records {
		if rec == expectedVal {
			matched = true
			break
		}
	}

	if matched {
		return s.repo.UpdateDomainVerificationStatus(ctx, domainID, "verified", &now, &now)
	}

	return s.repo.UpdateDomainVerificationStatus(ctx, domainID, "failed", nil, &now)
}

// ActivateCustomStoreDomainForSubject promotes a verified custom domain to active primary.
func (s Service) ActivateCustomStoreDomainForSubject(ctx context.Context, subject, storeID, domainID string) (StoreDomain, error) {
	store, err := s.repo.GetStore(ctx, storeID)
	if err != nil {
		return StoreDomain{}, err
	}
	if _, err := s.RequireSellerAccess(ctx, subject, store.SellerID); err != nil {
		return StoreDomain{}, err
	}
	return s.repo.ActivateCustomDomainTx(ctx, storeID, domainID)
}

// AdminListDomains lists domains across stores for platform administration.
func (s Service) AdminListDomains(ctx context.Context, filter AdminDomainFilter) ([]StoreDomain, error) {
	return s.repo.ListDomainsAdmin(ctx, filter)
}

// AdminDisableDomain disables a domain for moderation.
func (s Service) AdminDisableDomain(ctx context.Context, domainID string) (StoreDomain, error) {
	return s.repo.DisableDomainTx(ctx, domainID)
}

// AdminEnableDomain re-enables a domain for moderation.
func (s Service) AdminEnableDomain(ctx context.Context, domainID string) (StoreDomain, error) {
	return s.repo.EnableDomainTx(ctx, domainID)
}
