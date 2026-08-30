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
