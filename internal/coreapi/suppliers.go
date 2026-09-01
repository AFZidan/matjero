package coreapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/matjeroapps/core/internal/serviceauth"
	"github.com/matjeroapps/core/packages/httpx"
	"github.com/matjeroapps/core/packages/money"
	"github.com/matjeroapps/core/pkg/commerce"
)

// Supplier handlers.
//
// As with sellers, business identity is resolved by Core from the forwarded
// subject. Every supplier-scoped write goes through a *ForSubject service
// method so ownership is enforced in the domain layer, not by the transport.

func (s *server) handleResolveSupplier(w http.ResponseWriter, r *http.Request) {
	subject := serviceauth.SubjectFrom(r)
	if subject == "" {
		writeError(w, CodeInvalidArgument)
		return
	}
	supplierID, err := s.deps.Commerce.ResolveSupplierIDForSubject(r.Context(), subject)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, SupplierResolveResponse{SupplierID: supplierID})
}

func (s *server) handleGetSupplier(w http.ResponseWriter, r *http.Request) {
	supplierID, ok := s.authorizeSupplier(w, r)
	if !ok {
		return
	}
	supplier, err := s.deps.Repo.GetSupplierByID(r.Context(), supplierID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	settings, _ := s.deps.Repo.GetSupplierSettings(r.Context(), supplierID)
	httpx.WriteJSON(w, http.StatusOK, SupplierProfileResponse{Supplier: supplier, Settings: settings})
}

func (s *server) handleUpdateSupplierProfile(w http.ResponseWriter, r *http.Request) {
	supplierID, ok := s.authorizeSupplier(w, r)
	if !ok {
		return
	}
	var body ProfileUpdateRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.deps.Repo.UpdateSupplierProfile(r.Context(), supplierID, body.Name, body.Status, body.Settings); err != nil {
		writeDomainError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, StatusResponse{Status: body.Status})
}

func (s *server) handleUpdateSupplierStatus(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.deps.Repo.UpdateSupplierStatus(r.Context(), chi.URLParam(r, "supplierID"), body.Status); err != nil {
		writeDomainError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, StatusResponse{Status: body.Status})
}

func (s *server) handleListSuppliers(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	items, err := s.deps.Repo.ListSuppliers(r.Context(), parsePage(r))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, CollectionResponse[commerce.Supplier]{Items: items})
}

func (s *server) handleListSupplierMarkets(w http.ResponseWriter, r *http.Request) {
	supplierID, ok := s.authorizeSupplier(w, r)
	if !ok {
		return
	}
	items, err := s.deps.Repo.ListSupplierMarkets(r.Context(), supplierID, parsePage(r))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, CollectionResponse[commerce.SupplierMarket]{Items: items})
}

func (s *server) handleListSupplierLocations(w http.ResponseWriter, r *http.Request) {
	supplierID, ok := s.authorizeSupplier(w, r)
	if !ok {
		return
	}
	items, err := s.deps.Repo.ListFulfillmentLocations(r.Context(), supplierID, parsePage(r))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, CollectionResponse[commerce.FulfillmentLocation]{Items: items})
}

func (s *server) handleCreateSupplierLocation(w http.ResponseWriter, r *http.Request) {
	subject, supplierID, ok := s.authorizeSupplierSubject(w, r)
	if !ok {
		return
	}
	var body FulfillmentLocationCreateRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	location, err := s.deps.Commerce.CreateFulfillmentLocationForSubject(r.Context(), subject, supplierID, body.SupplierMarketID, body.MarketCode, body.Code, body.Name, body.LocationType, body.Status)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, location)
}

func (s *server) handleListSupplierProducts(w http.ResponseWriter, r *http.Request) {
	supplierID, ok := s.authorizeSupplier(w, r)
	if !ok {
		return
	}
	items, err := s.deps.Repo.ListSupplierProducts(r.Context(), supplierID, parsePage(r))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, CollectionResponse[commerce.SupplierProduct]{Items: items})
}

// handleCreateSupplierProduct creates the global product, its translations, the
// supplier binding, and the category assignments.
//
// The sequence matches the supplier API's existing behaviour exactly: the
// product is created first, translations are upserted, the supplier binding is
// created through the ownership-enforcing service method, and categories are
// assigned last.
func (s *server) handleCreateSupplierProduct(w http.ResponseWriter, r *http.Request) {
	subject, supplierID, ok := s.authorizeSupplierSubject(w, r)
	if !ok {
		return
	}
	var body ProductCreateRequest
	if !decodeJSON(w, r, &body) {
		return
	}

	product, err := s.deps.Repo.CreateProduct(r.Context(), body.Slug, body.Status)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	for _, translation := range body.Translations {
		if err := s.deps.Repo.UpsertProductTranslation(r.Context(), commerce.ProductTranslation{
			ProductID:   product.ID,
			Locale:      translation.Locale,
			Name:        translation.Name,
			Description: translation.Description,
		}); err != nil {
			writeDomainError(w, err)
			return
		}
	}
	supplierProduct, err := s.deps.Commerce.CreateSupplierProductForSubject(r.Context(), subject, supplierID, product.ID, body.SupplierCode, body.Status)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if len(body.CategoryIDs) > 0 {
		if err := s.deps.Repo.SetProductCategories(r.Context(), product.ID, body.CategoryIDs); err != nil {
			writeDomainError(w, err)
			return
		}
	}
	httpx.WriteJSON(w, http.StatusCreated, ProductCreateResponse{Product: product, SupplierProduct: supplierProduct})
}

func (s *server) handleSetProductCategories(w http.ResponseWriter, r *http.Request) {
	subject, supplierID, ok := s.authorizeSupplierSubject(w, r)
	if !ok {
		return
	}
	var body ProductCategoriesRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.deps.Commerce.SetProductCategoriesForSubject(r.Context(), subject, supplierID, chi.URLParam(r, "productID"), body.CategoryIDs); err != nil {
		writeDomainError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ProductCategoriesResponse{CategoryIDs: body.CategoryIDs})
}

func (s *server) handleListSupplierOffers(w http.ResponseWriter, r *http.Request) {
	supplierID, ok := s.authorizeSupplier(w, r)
	if !ok {
		return
	}
	items, err := s.deps.Repo.ListSupplierOffers(r.Context(), supplierID, parsePage(r))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, CollectionResponse[commerce.SupplierOffer]{Items: items})
}

// handleCreateSupplierOffer creates an offer and optionally seeds its price and
// availability, matching the supplier API's existing two-step behaviour.
func (s *server) handleCreateSupplierOffer(w http.ResponseWriter, r *http.Request) {
	subject, supplierID, ok := s.authorizeSupplierSubject(w, r)
	if !ok {
		return
	}
	var body SupplierOfferCreateRequest
	if !decodeJSON(w, r, &body) {
		return
	}

	offer, err := s.deps.Commerce.CreateSupplierOfferForSubject(r.Context(), subject, supplierID, body.SupplierProductID, body.SupplierMarketID, body.MarketCode, body.Status)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if body.Price != nil {
		price, err := money.New(body.Price.AmountMinor, body.Price.Currency)
		if err != nil {
			writeError(w, CodeValidationError)
			return
		}
		if _, err := s.deps.Repo.SetSupplierOfferPrice(r.Context(), offer.ID, price); err != nil {
			writeDomainError(w, err)
			return
		}
	}
	if body.IsAvailable != nil || body.AvailableQty != nil {
		isAvailable := body.IsAvailable != nil && *body.IsAvailable
		if _, err := s.deps.Repo.SetSupplierOfferAvailability(r.Context(), offer.ID, isAvailable, body.AvailableQty); err != nil {
			writeDomainError(w, err)
			return
		}
	}
	httpx.WriteJSON(w, http.StatusCreated, offer)
}

func (s *server) handleListInventorySnapshots(w http.ResponseWriter, r *http.Request) {
	supplierID, ok := s.authorizeSupplier(w, r)
	if !ok {
		return
	}
	items, err := s.deps.Repo.ListInventorySnapshots(r.Context(), supplierID, parsePage(r))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, CollectionResponse[commerce.InventorySnapshot]{Items: items})
}

// handleCreateInventorySnapshot opens a snapshot after verifying the target
// fulfillment location belongs to the authenticated supplier.
func (s *server) handleCreateInventorySnapshot(w http.ResponseWriter, r *http.Request) {
	_, supplierID, ok := s.authorizeSupplierSubject(w, r)
	if !ok {
		return
	}
	var body InventorySnapshotCreateRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	location, err := s.deps.Repo.GetFulfillmentLocationByID(r.Context(), body.FulfillmentLocationID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if location.SupplierID != supplierID {
		writeError(w, CodeNotFound)
		return
	}
	snapshot, err := s.deps.Repo.CreateInventorySnapshot(r.Context(), body.FulfillmentLocationID, body.SKUID, body.OnHandQty)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, snapshot)
}

// handleAdjustInventory applies a stock movement. Correlation and causation
// identifiers are taken from the request context so the movement stays traceable
// back to the originating actor request.
func (s *server) handleAdjustInventory(w http.ResponseWriter, r *http.Request) {
	subject, supplierID, ok := s.authorizeSupplierSubject(w, r)
	if !ok {
		return
	}
	var body InventoryAdjustmentRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	snapshot, movement, err := s.deps.Commerce.AdjustInventoryForSubject(
		r.Context(), subject, supplierID, chi.URLParam(r, "snapshotID"),
		body.QuantityDelta, body.MovementType, body.Reason,
		httpx.CorrelationID(r.Context()), httpx.RequestID(r.Context()),
	)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, InventoryAdjustmentResponse{Snapshot: snapshot, Movement: movement})
}

func (s *server) handleListInventoryMovements(w http.ResponseWriter, r *http.Request) {
	// The caller must be the owning supplier (or admin); the snapshot is then
	// addressed directly by its own identifier.
	if _, ok := s.authorizeSupplier(w, r); !ok {
		return
	}
	items, err := s.deps.Repo.ListInventoryMovements(r.Context(), chi.URLParam(r, "snapshotID"), parsePage(r))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, CollectionResponse[commerce.InventoryMovement]{Items: items})
}

// authorizeSupplier resolves the supplier a request may act on. Admin may name
// any supplier; a supplier caller is always scoped to its own identity.
func (s *server) authorizeSupplier(w http.ResponseWriter, r *http.Request) (string, bool) {
	_, supplierID, ok := s.authorizeSupplierSubject(w, r)
	return supplierID, ok
}

// authorizeSupplierSubject additionally returns the forwarded subject, which the
// ownership-enforcing service methods require.
func (s *server) authorizeSupplierSubject(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	caller, ok := serviceauth.CallerFrom(r.Context())
	if !ok {
		writeError(w, CodeUnauthorized)
		return "", "", false
	}

	if caller == serviceauth.CallerAdmin {
		return serviceauth.SubjectFrom(r), chi.URLParam(r, "supplierID"), true
	}

	subject := serviceauth.SubjectFrom(r)
	if subject == "" {
		writeError(w, CodeInvalidArgument)
		return "", "", false
	}
	supplierID, err := s.deps.Commerce.ResolveSupplierIDForSubject(r.Context(), subject)
	if err != nil {
		writeDomainError(w, err)
		return "", "", false
	}
	if supplierID != chi.URLParam(r, "supplierID") {
		writeError(w, CodeNotFound)
		return "", "", false
	}
	return subject, supplierID, true
}
