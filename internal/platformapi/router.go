package platformapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"dropshipping/internal/commerce"
	"dropshipping/packages/auth"
	"dropshipping/packages/httpx"
	"dropshipping/packages/i18n"
	"dropshipping/packages/money"
)

type Dependencies struct {
	Commerce commerce.Service
	Repo     commerce.Repository
}

func RegisterAdminRoutes(deps Dependencies) func(r chi.Router) {
	return func(r chi.Router) {
		r.Get("/admin/overview", deps.handleAdminOverview)
		r.Get("/admin/suppliers", deps.handleAdminSuppliers)
		r.Post("/admin/suppliers/{id}/status", deps.handleAdminSupplierStatus)
		r.Get("/admin/sellers", deps.handleAdminSellers)
		r.Post("/admin/sellers/{id}/status", deps.handleAdminSellerStatus)
		r.Get("/admin/stores", deps.handleAdminStores)
		r.Post("/admin/stores/{id}/status", deps.handleAdminStoreStatus)
		r.Get("/admin/products", deps.handleAdminProducts)
		r.Post("/admin/products/{id}/status", deps.handleAdminProductStatus)
		r.Get("/admin/categories", deps.handleAdminCategories)
		r.Post("/admin/categories/{id}/status", deps.handleAdminCategoryStatus)
		r.Get("/admin/offers", deps.handleAdminOffers)
		r.Post("/admin/offers/{id}/status", deps.handleAdminOfferStatus)
		r.Get("/admin/listings", deps.handleAdminListings)
		r.Post("/admin/listings/{id}/status", deps.handleAdminListingStatus)
		r.Get("/admin/locations", deps.handleAdminLocations)
		r.Post("/admin/locations/{id}/status", deps.handleAdminLocationStatus)
	}
}

func RegisterSupplierRoutes(deps Dependencies) func(r chi.Router) {
	return func(r chi.Router) {
		r.Get("/supplier/profile", deps.handleSupplierProfile)
		r.Put("/supplier/profile", deps.handleSupplierProfileUpdate)
		r.Get("/supplier/markets", deps.handleSupplierMarkets)
		r.Get("/supplier/locations", deps.handleSupplierLocations)
		r.Post("/supplier/locations", deps.handleSupplierLocationCreate)
		r.Get("/supplier/products", deps.handleSupplierProducts)
		r.Post("/supplier/products", deps.handleSupplierProductCreate)
		r.Put("/supplier/products/{id}/categories", deps.handleSupplierProductCategories)
		r.Get("/supplier/offers", deps.handleSupplierOffers)
		r.Post("/supplier/offers", deps.handleSupplierOfferCreate)
		r.Get("/supplier/inventory", deps.handleSupplierInventory)
		r.Post("/supplier/inventory/snapshots", deps.handleSupplierInventorySnapshotCreate)
		r.Post("/supplier/inventory/{snapshot_id}/adjustments", deps.handleSupplierInventoryAdjustment)
		r.Get("/supplier/inventory/{snapshot_id}/movements", deps.handleSupplierInventoryMovements)
	}
}

func RegisterSellerRoutes(deps Dependencies) func(r chi.Router) {
	return func(r chi.Router) {
		r.Get("/seller/profile", deps.handleSellerProfile)
		r.Put("/seller/profile", deps.handleSellerProfileUpdate)
		r.Get("/seller/stores", deps.handleSellerStores)
		r.Post("/seller/stores", deps.handleSellerStoreCreate)
		r.Get("/seller/catalog/offers", deps.handleSellerCatalogOffers)
		r.Get("/seller/listings", deps.handleSellerListings)
		r.Post("/seller/listings/import", deps.handleSellerListingImport)
		r.Post("/seller/listings/{id}/price", deps.handleSellerListingPrice)
		r.Post("/seller/listings/{id}/status", deps.handleSellerListingStatus)
	}
}

type pageQuery struct {
	Limit  int
	Offset int
}

func parsePage(r *http.Request) pageQuery {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}
	return pageQuery{Limit: limit, Offset: offset}
}

func subjectFrom(r *http.Request) (string, error) {
	principal, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		return "", errors.New("missing principal")
	}
	if principal.Subject == "" {
		return "", errors.New("missing principal subject")
	}
	return principal.Subject, nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return false
	}
	return true
}

func (deps Dependencies) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var counts map[string]int
	tables := []struct {
		key   string
		query string
	}{
		{"suppliers", "SELECT count(*) FROM suppliers"},
		{"sellers", "SELECT count(*) FROM sellers"},
		{"stores", "SELECT count(*) FROM stores"},
		{"products", "SELECT count(*) FROM products"},
		{"categories", "SELECT count(*) FROM categories"},
		{"offers", "SELECT count(*) FROM supplier_offers"},
		{"listings", "SELECT count(*) FROM seller_listings"},
	}
	counts = make(map[string]int, len(tables))
	for _, table := range tables {
		var count int
		if err := deps.Repo.Pool().QueryRow(ctx, table.query).Scan(&count); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "overview_unavailable", "overview unavailable")
			return
		}
		counts[table.key] = count
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"counts": counts})
}

func (deps Dependencies) handleAdminSuppliers(w http.ResponseWriter, r *http.Request) {
	items, err := deps.Repo.ListSuppliers(r.Context(), commerce.Page(parsePage(r)))
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (deps Dependencies) handleAdminSellers(w http.ResponseWriter, r *http.Request) {
	items, err := deps.Repo.ListSellers(r.Context(), commerce.Page(parsePage(r)))
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (deps Dependencies) handleAdminStores(w http.ResponseWriter, r *http.Request) {
	items, err := deps.Repo.ListStores(r.Context(), commerce.Page(parsePage(r)))
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (deps Dependencies) handleAdminProducts(w http.ResponseWriter, r *http.Request) {
	items, err := deps.Repo.ListProducts(r.Context(), commerce.Page(parsePage(r)))
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (deps Dependencies) handleAdminCategories(w http.ResponseWriter, r *http.Request) {
	items, err := deps.Repo.ListCategories(r.Context(), commerce.Page(parsePage(r)))
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (deps Dependencies) handleAdminOffers(w http.ResponseWriter, r *http.Request) {
	filter := commerce.SupplierCatalogFilter{
		Page: commerce.Page(parsePage(r)),
	}
	if market := r.URL.Query().Get("market_code"); market != "" {
		filter.MarketCode = market
	}
	items, err := deps.Repo.ListSupplierCatalog(r.Context(), filter)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (deps Dependencies) handleAdminListings(w http.ResponseWriter, r *http.Request) {
	page := commerce.Page(parsePage(r))
	items, err := deps.Repo.ListSellerListings(r.Context(), r.URL.Query().Get("store_id"), page)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (deps Dependencies) handleAdminLocations(w http.ResponseWriter, r *http.Request) {
	items, err := deps.Repo.ListFulfillmentLocations(r.Context(), r.URL.Query().Get("supplier_id"), commerce.Page(parsePage(r)))
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (deps Dependencies) handleAdminSupplierStatus(w http.ResponseWriter, r *http.Request) {
	var body struct{ Status string `json:"status"` }
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := deps.Repo.UpdateSupplierStatus(r.Context(), chi.URLParam(r, "id"), body.Status); err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": body.Status})
}

func (deps Dependencies) handleAdminSellerStatus(w http.ResponseWriter, r *http.Request) {
	var body struct{ Status string `json:"status"` }
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := deps.Repo.UpdateSellerStatus(r.Context(), chi.URLParam(r, "id"), body.Status); err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": body.Status})
}

func (deps Dependencies) handleAdminStoreStatus(w http.ResponseWriter, r *http.Request) { updateStatusHandler(w, r, deps.Repo.UpdateStoreStatus) }
func (deps Dependencies) handleAdminProductStatus(w http.ResponseWriter, r *http.Request) { updateStatusHandler(w, r, deps.Repo.UpdateProductStatus) }
func (deps Dependencies) handleAdminCategoryStatus(w http.ResponseWriter, r *http.Request) { updateStatusHandler(w, r, deps.Repo.UpdateCategoryStatus) }
func (deps Dependencies) handleAdminOfferStatus(w http.ResponseWriter, r *http.Request) { updateStatusHandler(w, r, deps.Repo.UpdateSupplierOfferStatus) }
func (deps Dependencies) handleAdminListingStatus(w http.ResponseWriter, r *http.Request) { updateStatusHandler(w, r, deps.Repo.UpdateSellerListingStatus) }
func (deps Dependencies) handleAdminLocationStatus(w http.ResponseWriter, r *http.Request) { updateStatusHandler(w, r, deps.Repo.UpdateFulfillmentLocationStatus) }

func (deps Dependencies) handleSupplierProfile(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	supplierID, err := resolveSupplierID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	supplier, err := deps.Repo.GetSupplierByID(r.Context(), supplierID)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	settings, _ := deps.Repo.GetSupplierSettings(r.Context(), supplier.ID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"supplier": supplier, "settings": settings})
}

func (deps Dependencies) handleSupplierProfileUpdate(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	supplierID, err := resolveSupplierID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	var body struct {
		Name     string         `json:"name"`
		Status   string         `json:"status"`
		Settings map[string]any `json:"settings"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := deps.Repo.UpdateSupplierProfile(r.Context(), supplierID, body.Name, body.Status, body.Settings); err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": body.Status})
}

func (deps Dependencies) handleSupplierMarkets(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	supplierID, err := resolveSupplierID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	items, err := deps.Repo.ListSupplierMarkets(r.Context(), supplierID, commerce.Page(parsePage(r)))
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (deps Dependencies) handleSupplierLocations(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	supplierID, err := resolveSupplierID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	items, err := deps.Repo.ListFulfillmentLocations(r.Context(), supplierID, commerce.Page(parsePage(r)))
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (deps Dependencies) handleSupplierLocationCreate(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	var body struct {
		SupplierMarketID string `json:"supplier_market_id"`
		MarketCode       string `json:"market_code"`
		Code             string `json:"code"`
		Name             string `json:"name"`
		LocationType     string `json:"location_type"`
		Status           string `json:"status"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	supplierID, err := resolveSupplierID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	location, err := deps.Commerce.CreateFulfillmentLocationForSubject(r.Context(), subject, supplierID, body.SupplierMarketID, body.MarketCode, body.Code, body.Name, body.LocationType, body.Status)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, location)
}

func (deps Dependencies) handleSupplierProducts(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	supplierID, err := resolveSupplierID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	items, err := deps.Repo.ListSupplierProducts(r.Context(), supplierID, commerce.Page(parsePage(r)))
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (deps Dependencies) handleSupplierProductCreate(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	var body struct {
		Slug        string              `json:"slug"`
		Status      string              `json:"status"`
		SupplierCode string             `json:"supplier_code"`
		Translations []translationInput `json:"translations"`
		CategoryIDs  []string           `json:"category_ids"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	supplierID, err := resolveSupplierID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	product, err := deps.Repo.CreateProduct(r.Context(), body.Slug, body.Status)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	for _, translation := range body.Translations {
		if err := deps.Repo.UpsertProductTranslation(r.Context(), commerce.ProductTranslation{ProductID: product.ID, Locale: translation.Locale, Name: translation.Name, Description: translation.Description}); err != nil {
			writeCommerceError(w, err)
			return
		}
	}
	supplierProduct, err := deps.Commerce.CreateSupplierProductForSubject(r.Context(), subject, supplierID, product.ID, body.SupplierCode, body.Status)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	if len(body.CategoryIDs) > 0 {
		if err := deps.Repo.SetProductCategories(r.Context(), product.ID, body.CategoryIDs); err != nil {
			writeCommerceError(w, err)
			return
		}
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"product": product, "supplier_product": supplierProduct})
}

func (deps Dependencies) handleSupplierProductCategories(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	supplierID, err := resolveSupplierID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	var body struct {
		CategoryIDs []string `json:"category_ids"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	productID := chi.URLParam(r, "id")
	if err := deps.Commerce.SetProductCategoriesForSubject(r.Context(), subject, supplierID, productID, body.CategoryIDs); err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"category_ids": body.CategoryIDs})
}

func (deps Dependencies) handleSupplierOffers(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	supplierID, err := resolveSupplierID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	items, err := deps.Repo.ListSupplierOffers(r.Context(), supplierID, commerce.Page(parsePage(r)))
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (deps Dependencies) handleSupplierOfferCreate(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	supplierID, err := resolveSupplierID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	var body struct {
		SupplierProductID string `json:"supplier_product_id"`
		SupplierMarketID  string `json:"supplier_market_id"`
		MarketCode        string `json:"market_code"`
		Status            string `json:"status"`
		Price             *money.Money `json:"price"`
		IsAvailable       *bool        `json:"is_available"`
		AvailableQty      *int64       `json:"available_qty"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	offer, err := deps.Commerce.CreateSupplierOfferForSubject(r.Context(), subject, supplierID, body.SupplierProductID, body.SupplierMarketID, body.MarketCode, body.Status)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	if body.Price != nil {
		if _, err := deps.Repo.SetSupplierOfferPrice(r.Context(), offer.ID, *body.Price); err != nil {
			writeCommerceError(w, err)
			return
		}
	}
	if body.IsAvailable != nil || body.AvailableQty != nil {
		_, err := deps.Repo.SetSupplierOfferAvailability(r.Context(), offer.ID, body.IsAvailable != nil && *body.IsAvailable, body.AvailableQty)
		if err != nil {
			writeCommerceError(w, err)
			return
		}
	}
	httpx.WriteJSON(w, http.StatusCreated, offer)
}

func (deps Dependencies) handleSupplierInventory(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	supplierID, err := resolveSupplierID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	items, err := deps.Repo.ListInventorySnapshots(r.Context(), supplierID, commerce.Page(parsePage(r)))
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (deps Dependencies) handleSupplierInventorySnapshotCreate(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	var body struct {
		FulfillmentLocationID string `json:"fulfillment_location_id"`
		SKUID                string `json:"sku_id"`
		OnHandQty            int64  `json:"on_hand_qty"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	supplierID, err := resolveSupplierID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	location, err := deps.Repo.GetFulfillmentLocationByID(r.Context(), body.FulfillmentLocationID)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	if location.SupplierID != supplierID {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "resource not found")
		return
	}
	snapshot, err := deps.Repo.CreateInventorySnapshot(r.Context(), body.FulfillmentLocationID, body.SKUID, body.OnHandQty)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, snapshot)
}

func (deps Dependencies) handleSupplierInventoryAdjustment(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	supplierID, err := resolveSupplierID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	var body struct {
		QuantityDelta int64  `json:"quantity_delta"`
		MovementType  string `json:"movement_type"`
		Reason        string `json:"reason"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	snapshotID := chi.URLParam(r, "snapshot_id")
	snapshot, movement, err := deps.Commerce.AdjustInventoryForSubject(r.Context(), subject, supplierID, snapshotID, body.QuantityDelta, body.MovementType, body.Reason, httpx.CorrelationID(r.Context()), httpx.RequestID(r.Context()))
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"snapshot": snapshot, "movement": movement})
}

func (deps Dependencies) handleSupplierInventoryMovements(w http.ResponseWriter, r *http.Request) {
	items, err := deps.Repo.ListInventoryMovements(r.Context(), chi.URLParam(r, "snapshot_id"), commerce.Page(parsePage(r)))
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (deps Dependencies) handleSellerProfile(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	sellerID, err := resolveSellerID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	seller, err := deps.Repo.GetSellerByID(r.Context(), sellerID)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	settings, _ := deps.Repo.GetSellerSettings(r.Context(), sellerID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"seller": seller, "settings": settings})
}

func (deps Dependencies) handleSellerProfileUpdate(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	sellerID, err := resolveSellerID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	var body struct {
		Name     string         `json:"name"`
		Status   string         `json:"status"`
		Settings map[string]any `json:"settings"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := deps.Repo.UpdateSellerProfile(r.Context(), sellerID, body.Name, body.Status, body.Settings); err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": body.Status})
}

func (deps Dependencies) handleSellerStores(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	sellerID, err := resolveSellerID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	items, err := deps.Repo.ListStores(r.Context(), commerce.Page(parsePage(r)))
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	filtered := items[:0]
	for _, item := range items {
		if item.SellerID == sellerID {
			filtered = append(filtered, item)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": filtered})
}

func (deps Dependencies) handleSellerStoreCreate(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	sellerID, err := resolveSellerID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	var body struct {
		MarketCode string         `json:"market_code"`
		Code       string         `json:"code"`
		Name       string         `json:"name"`
		Status     string         `json:"status"`
		Settings   map[string]any `json:"settings"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	store, err := deps.Commerce.CreateStoreForSubject(r.Context(), subject, sellerID, body.MarketCode, body.Code, body.Name, body.Status, body.Settings)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, store)
}

func (deps Dependencies) handleSellerCatalogOffers(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	sellerID, err := resolveSellerID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	storeID := r.URL.Query().Get("store_id")
	store, err := deps.Repo.GetStore(r.Context(), storeID)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	if store.SellerID != sellerID {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "resource not found")
		return
	}
	filter := commerce.SupplierCatalogFilter{
		MarketCode: store.MarketCode,
		Locale:     string(i18n.FromContext(r.Context())),
		Page:       commerce.Page(parsePage(r)),
	}
	if supplier := r.URL.Query().Get("supplier_id"); supplier != "" {
		filter.SupplierID = supplier
	}
	if category := r.URL.Query().Get("category_id"); category != "" {
		filter.CategoryID = category
	}
	items, err := deps.Repo.ListSupplierCatalog(r.Context(), filter)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (deps Dependencies) handleSellerListings(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	sellerID, err := resolveSellerID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	storeID := r.URL.Query().Get("store_id")
	store, err := deps.Repo.GetStore(r.Context(), storeID)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	if store.SellerID != sellerID {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "resource not found")
		return
	}
	items, err := deps.Repo.ListSellerListings(r.Context(), storeID, commerce.Page(parsePage(r)))
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (deps Dependencies) handleSellerListingImport(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFrom(r)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	sellerID, err := resolveSellerID(r.Context(), deps.Commerce, subject)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	var body struct {
		StoreID        string  `json:"store_id"`
		ProductID      string  `json:"product_id"`
		SupplierOfferID *string `json:"supplier_offer_id"`
		Status         string  `json:"status"`
		MarketCode     string  `json:"market_code"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	store, err := deps.Repo.GetStore(r.Context(), body.StoreID)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	if store.SellerID != sellerID {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "resource not found")
		return
	}
	listing, err := deps.Commerce.CreateSellerListingForSubject(r.Context(), subject, body.StoreID, body.ProductID, body.SupplierOfferID, body.MarketCode, body.Status)
	if err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, listing)
}

func (deps Dependencies) handleSellerListingPrice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AmountMinor int64  `json:"amount_minor"`
		Currency    string `json:"currency"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	price, err := money.New(body.AmountMinor, body.Currency)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if _, err := deps.Repo.SetSellerListingPrice(r.Context(), chi.URLParam(r, "id"), price); err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (deps Dependencies) handleSellerListingStatus(w http.ResponseWriter, r *http.Request) {
	updateStatusHandler(w, r, deps.Repo.UpdateSellerListingStatus)
}

type translationInput struct {
	Locale      string `json:"locale"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func updateStatusHandler(w http.ResponseWriter, r *http.Request, fn func(context.Context, string, string) error) {
	var body struct{ Status string `json:"status"` }
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := fn(r.Context(), chi.URLParam(r, "id"), body.Status); err != nil {
		writeCommerceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": body.Status})
}

func resolveSupplierID(ctx context.Context, svc commerce.Service, subject string) (string, error) {
	supplierID, err := svc.ResolveSupplierIDForSubject(ctx, subject)
	if err != nil {
		return "", err
	}
	return supplierID, nil
}

func resolveSellerID(ctx context.Context, svc commerce.Service, subject string) (string, error) {
	sellerID, err := svc.ResolveSellerIDForSubject(ctx, subject)
	if err != nil {
		return "", err
	}
	return sellerID, nil
}

func writeCommerceError(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		return
	case errors.Is(err, commerce.ErrInvalidInput):
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", "invalid input")
	case errors.Is(err, commerce.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, commerce.ErrConflict):
		httpx.WriteError(w, http.StatusConflict, "conflict", "conflict")
	case errors.Is(err, commerce.ErrMarketMismatch):
		httpx.WriteError(w, http.StatusConflict, "market_mismatch", "market mismatch")
	case errors.Is(err, commerce.ErrInsufficientInventory):
		httpx.WriteError(w, http.StatusConflict, "insufficient_inventory", "insufficient inventory")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
