package commerce

import (
	"context"
	"fmt"
	"time"
)

type Service struct {
	repo Repository
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
	return s.repo.CreateStore(ctx, sellerID, marketCode, code, name, status, settings)
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
