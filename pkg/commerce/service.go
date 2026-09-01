package commerce

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Service struct {
	repo Repository

	// PlatformDomain is the base domain under which platform-generated store
	// subdomains are allocated (e.g. "<store-code>.matjero.com"). When empty, no
	// subdomain is auto-allocated at store creation.
	PlatformDomain string
	// ReservedSubdomains are store-code labels that may not be claimed by sellers
	// because they are reserved for platform use.
	ReservedSubdomains []string
}

func NewService(repo Repository) Service {
	return Service{repo: repo}
}

func (s Service) CreateSellerListing(ctx context.Context, storeID, productID string, supplierOfferID *string, marketCode, status string) (SellerListing, error) {
	store, err := s.repo.GetStore(ctx, storeID)
	if err != nil {
		return SellerListing{}, err
	}
	if store.MarketCode != marketCode {
		return SellerListing{}, fmt.Errorf("%w: store market %s does not match %s", ErrMarketMismatch, store.MarketCode, marketCode)
	}
	if supplierOfferID != nil {
		offer, err := s.repo.GetSupplierOffer(ctx, *supplierOfferID)
		if err != nil {
			return SellerListing{}, err
		}
		if offer.MarketCode != marketCode {
			return SellerListing{}, fmt.Errorf("%w: supplier offer market %s does not match %s", ErrMarketMismatch, offer.MarketCode, marketCode)
		}
	}
	return s.repo.CreateSellerListing(ctx, storeID, productID, supplierOfferID, marketCode, status)
}

func (s Service) ReserveInventory(ctx context.Context, snapshotID string, quantity int64, reservationToken string, expiresAt *time.Time) (InventoryReservation, error) {
	if quantity <= 0 {
		return InventoryReservation{}, ErrInvalidInput
	}
	return s.repo.ReserveInventory(ctx, snapshotID, quantity, reservationToken, expiresAt)
}

func (s Service) RequireSupplierAccess(ctx context.Context, subject, supplierID string) (Supplier, error) {
	if supplierID == "" {
		return Supplier{}, ErrInvalidInput
	}
	supplier, err := s.repo.GetSupplierForSubject(ctx, subject)
	if err != nil {
		return Supplier{}, err
	}
	if supplier.ID != supplierID {
		return Supplier{}, ErrNotFound
	}
	return supplier, nil
}

func (s Service) RequireSellerAccess(ctx context.Context, subject, sellerID string) (Seller, error) {
	if sellerID == "" {
		return Seller{}, ErrInvalidInput
	}
	seller, err := s.repo.GetSellerForSubject(ctx, subject)
	if err != nil {
		return Seller{}, err
	}
	if seller.ID != sellerID {
		return Seller{}, ErrNotFound
	}
	return seller, nil
}

func (s Service) CreateStoreForSubject(ctx context.Context, subject, sellerID, marketCode, code, name, status string, settings map[string]any) (Store, error) {
	if _, err := s.RequireSellerAccess(ctx, subject, sellerID); err != nil {
		return Store{}, err
	}

	// No platform domain configured: create the store without an auto-allocated
	// subdomain.
	if s.PlatformDomain == "" {
		return s.repo.CreateStore(ctx, sellerID, marketCode, code, name, status, settings)
	}

	// Validate the reserved subdomain before any database write. The platform
	// subdomain is derived from the store code, so a reserved code must be
	// rejected up front.
	normalizedCode := strings.ToLower(strings.TrimSpace(code))
	for _, reserved := range s.ReservedSubdomains {
		if normalizedCode == strings.ToLower(strings.TrimSpace(reserved)) {
			return Store{}, ErrInvalidInput
		}
	}

	subdomain, err := NormalizeDomain(fmt.Sprintf("%s.%s", normalizedCode, s.PlatformDomain))
	if err != nil {
		return Store{}, ErrInvalidInput
	}

	now := time.Now()
	store, _, err := s.repo.CreateStoreWithDomain(ctx, sellerID, marketCode, code, name, status, settings, subdomain, "platform", "active", true, &now, nil)
	if err != nil {
		return Store{}, err
	}
	return store, nil
}

// RequestCustomStoreDomain registers a seller-supplied custom domain for a store
// in the PENDING lifecycle state. Authorization is enforced via the store's
// owning seller.
func (s Service) RequestCustomStoreDomain(ctx context.Context, subject, storeID, domain string) (StoreDomain, error) {
	store, err := s.repo.GetStore(ctx, storeID)
	if err != nil {
		return StoreDomain{}, err
	}
	if _, err := s.RequireSellerAccess(ctx, subject, store.SellerID); err != nil {
		return StoreDomain{}, err
	}
	return s.repo.CreateCustomStoreDomain(ctx, storeID, domain)
}

func (s Service) CreateFulfillmentLocationForSubject(ctx context.Context, subject, supplierID, supplierMarketID, marketCode, code, name, locationType, status string) (FulfillmentLocation, error) {
	if _, err := s.RequireSupplierAccess(ctx, subject, supplierID); err != nil {
		return FulfillmentLocation{}, err
	}
	return s.repo.CreateFulfillmentLocation(ctx, supplierID, supplierMarketID, marketCode, code, name, locationType, status)
}

func (s Service) CreateSupplierProductForSubject(ctx context.Context, subject, supplierID, productID, supplierCode, status string) (SupplierProduct, error) {
	if _, err := s.RequireSupplierAccess(ctx, subject, supplierID); err != nil {
		return SupplierProduct{}, err
	}
	return s.repo.CreateSupplierProduct(ctx, supplierID, productID, supplierCode, status)
}

// CreateSupplierProductWithDetailsForSubject creates a product, its translations,
// the supplier binding and the category assignments as one atomic operation.
//
// Callers that build these rows through separate repository calls can leave a
// product with no supplier binding, or a binding with no categories, when a later
// step fails. This method cannot.
func (s Service) CreateSupplierProductWithDetailsForSubject(ctx context.Context, subject, supplierID string, draft ProductDraft) (Product, SupplierProduct, error) {
	if _, err := s.RequireSupplierAccess(ctx, subject, supplierID); err != nil {
		return Product{}, SupplierProduct{}, err
	}
	return s.repo.CreateSupplierProductAtomically(ctx, supplierID, draft)
}

func (s Service) CreateSupplierOfferForSubject(ctx context.Context, subject, supplierID, supplierProductID, supplierMarketID, marketCode, status string) (SupplierOffer, error) {
	if _, err := s.RequireSupplierAccess(ctx, subject, supplierID); err != nil {
		return SupplierOffer{}, err
	}
	product, err := s.repo.GetSupplierProductByID(ctx, supplierProductID)
	if err != nil {
		return SupplierOffer{}, err
	}
	if product.SupplierID != supplierID {
		return SupplierOffer{}, ErrNotFound
	}
	market, err := s.repo.GetSupplierMarketByID(ctx, supplierMarketID)
	if err != nil {
		return SupplierOffer{}, err
	}
	if market.SupplierID != supplierID || market.MarketCode != marketCode {
		return SupplierOffer{}, ErrMarketMismatch
	}
	return s.repo.CreateSupplierOffer(ctx, supplierID, supplierProductID, supplierMarketID, marketCode, status)
}

// CreateSupplierOfferWithDetailsForSubject creates an offer together with its
// price and availability as one atomic operation, so an offer is never left
// unpriced because a later step failed.
func (s Service) CreateSupplierOfferWithDetailsForSubject(ctx context.Context, subject, supplierID string, draft OfferDraft) (SupplierOffer, error) {
	if _, err := s.RequireSupplierAccess(ctx, subject, supplierID); err != nil {
		return SupplierOffer{}, err
	}
	product, err := s.repo.GetSupplierProductByID(ctx, draft.SupplierProductID)
	if err != nil {
		return SupplierOffer{}, err
	}
	if product.SupplierID != supplierID {
		return SupplierOffer{}, ErrNotFound
	}
	market, err := s.repo.GetSupplierMarketByID(ctx, draft.SupplierMarketID)
	if err != nil {
		return SupplierOffer{}, err
	}
	if market.SupplierID != supplierID || market.MarketCode != draft.MarketCode {
		return SupplierOffer{}, ErrMarketMismatch
	}
	return s.repo.CreateSupplierOfferAtomically(ctx, supplierID, draft)
}

func (s Service) CreateSellerListingForSubject(ctx context.Context, subject, storeID, productID string, supplierOfferID *string, marketCode, status string) (SellerListing, error) {
	store, err := s.repo.GetStore(ctx, storeID)
	if err != nil {
		return SellerListing{}, err
	}
	if _, err := s.RequireSellerAccess(ctx, subject, store.SellerID); err != nil {
		return SellerListing{}, err
	}
	return s.CreateSellerListing(ctx, storeID, productID, supplierOfferID, marketCode, status)
}

func (s Service) SetProductCategoriesForSubject(ctx context.Context, subject, supplierID, productID string, categoryIDs []string) error {
	if _, err := s.RequireSupplierAccess(ctx, subject, supplierID); err != nil {
		return err
	}
	product, err := s.repo.GetSupplierProductBySupplierAndProduct(ctx, supplierID, productID)
	if err != nil {
		return err
	}
	if product.SupplierID != supplierID {
		return ErrNotFound
	}
	return s.repo.SetProductCategories(ctx, productID, categoryIDs)
}

func (s Service) AdjustInventoryForSubject(ctx context.Context, subject, supplierID, snapshotID string, quantityDelta int64, movementType, reason, correlationID, causationID string) (InventorySnapshot, InventoryMovement, error) {
	if _, err := s.RequireSupplierAccess(ctx, subject, supplierID); err != nil {
		return InventorySnapshot{}, InventoryMovement{}, err
	}
	snapshot, err := s.repo.GetInventorySnapshot(ctx, snapshotID)
	if err != nil {
		return InventorySnapshot{}, InventoryMovement{}, err
	}
	location, err := s.repo.GetFulfillmentLocationByID(ctx, snapshot.FulfillmentLocationID)
	if err != nil {
		return InventorySnapshot{}, InventoryMovement{}, err
	}
	if location.SupplierID != supplierID {
		return InventorySnapshot{}, InventoryMovement{}, ErrNotFound
	}
	return s.repo.AdjustInventory(ctx, snapshotID, quantityDelta, movementType, reason, subject, correlationID, causationID)
}

func (s Service) UpdateSupplierStatusForSubject(ctx context.Context, subject, supplierID, status string) error {
	if _, err := s.RequireSupplierAccess(ctx, subject, supplierID); err != nil {
		return err
	}
	return s.repo.UpdateSupplierStatus(ctx, supplierID, status)
}

func (s Service) UpdateSellerStatusForSubject(ctx context.Context, subject, sellerID, status string) error {
	if _, err := s.RequireSellerAccess(ctx, subject, sellerID); err != nil {
		return err
	}
	return s.repo.UpdateSellerStatus(ctx, sellerID, status)
}

func (s Service) ResolveSupplierIDForSubject(ctx context.Context, subject string) (string, error) {
	supplier, err := s.repo.GetSupplierForSubject(ctx, subject)
	if err != nil {
		return "", err
	}
	return supplier.ID, nil
}

func (s Service) ResolveSellerIDForSubject(ctx context.Context, subject string) (string, error) {
	seller, err := s.repo.GetSellerForSubject(ctx, subject)
	if err != nil {
		return "", err
	}
	return seller.ID, nil
}
