package openapi

import (
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"

	"dropshipping/internal/api"
	"dropshipping/internal/commerce"
	"dropshipping/internal/markets"
	"dropshipping/packages/httpx"
)

func BuildAdminSpec() (*openapi3.T, error) {
	return BuildDocument(DocumentSpec{
		Title:         "Matjero Admin API",
		Description:   "OpenAPI contract for the Matjero Admin API.",
		Authenticated: true,
		Tags:          openAPITags(),
		Routes:        append(actorRoutes(true), adminRoutes()...),
	})
}

func BuildSupplierSpec() (*openapi3.T, error) {
	return BuildDocument(DocumentSpec{
		Title:         "Matjero Supplier API",
		Description:   "OpenAPI contract for the Matjero Supplier API.",
		Authenticated: true,
		Tags:          openAPITags(),
		Routes:        append(actorRoutes(true), supplierRoutes()...),
	})
}

func BuildSellerSpec() (*openapi3.T, error) {
	return BuildDocument(DocumentSpec{
		Title:         "Matjero Seller API",
		Description:   "OpenAPI contract for the Matjero Seller API.",
		Authenticated: true,
		Tags:          openAPITags(),
		Routes:        append(actorRoutes(true), sellerRoutes()...),
	})
}

func BuildStorefrontSpec() (*openapi3.T, error) {
	return BuildDocument(DocumentSpec{
		Title:         "Matjero Storefront API",
		Description:   "OpenAPI contract for the public Matjero Storefront API.",
		Authenticated: false,
		Tags:          openAPITags(),
		Routes:        actorRoutes(false),
	})
}

func actorRoutes(authenticated bool) []RouteSpec {
	return []RouteSpec{
		{
			Method:      http.MethodGet,
			Path:        "/v1/bootstrap",
			OperationID: "getBootstrap",
			Summary:     "Load app bootstrap data",
			Description: "Returns the app identity, localization context, authenticated principal when present, and market list.",
			Tags:        []string{"Identity & Access", "Markets"},
			Auth:        authenticated,
			Responses: []ResponseSpec{
				okResponse("Bootstrap payload", api.Bootstrap{}),
				errorResponse(http.StatusUnauthorized, "Unauthorized"),
				errorResponse(http.StatusForbidden, "Forbidden"),
				errorResponse(http.StatusInternalServerError, "Internal error"),
			},
		},
		{
			Method:      http.MethodGet,
			Path:        "/v1/markets",
			OperationID: "listMarkets",
			Summary:     "List markets",
			Tags:        []string{"Markets"},
			Auth:        authenticated,
			Parameters:  []ParameterSpec{limitParam(), offsetParam()},
			Responses: []ResponseSpec{
				okResponse("Market collection", MarketsResponse{}),
				errorResponse(http.StatusUnauthorized, "Unauthorized"),
				errorResponse(http.StatusForbidden, "Forbidden"),
				errorResponse(http.StatusInternalServerError, "Internal error"),
			},
		},
		{
			Method:      http.MethodGet,
			Path:        "/v1/markets/{code}",
			OperationID: "getMarket",
			Summary:     "Get a market",
			Tags:        []string{"Markets"},
			Auth:        authenticated,
			Parameters:  []ParameterSpec{pathStringParam("code", "Market code")},
			Responses: []ResponseSpec{
				okResponse("Market", markets.Market{}),
				errorResponse(http.StatusUnauthorized, "Unauthorized"),
				errorResponse(http.StatusForbidden, "Forbidden"),
				errorResponse(http.StatusNotFound, "Not found"),
				errorResponse(http.StatusInternalServerError, "Internal error"),
			},
		},
	}
}

func adminRoutes() []RouteSpec {
	return []RouteSpec{
		{
			Method:      http.MethodGet,
			Path:        "/v1/admin/overview",
			OperationID: "getAdminOverview",
			Summary:     "Get platform overview",
			Description: "Returns high-level counts for major commerce aggregates.",
			Tags:        []string{"Audit"},
			Auth:        true,
			Responses: []ResponseSpec{
				okResponse("Platform overview", CountResponse{}),
				errorResponse(http.StatusUnauthorized, "Unauthorized"),
				errorResponse(http.StatusForbidden, "Forbidden"),
				errorResponse(http.StatusInternalServerError, "Internal error"),
			},
		},
		{
			Method:      http.MethodGet,
			Path:        "/v1/admin/suppliers",
			OperationID: "listAdminSuppliers",
			Summary:     "List suppliers",
			Tags:        []string{"Suppliers"},
			Auth:        true,
			Parameters:  []ParameterSpec{limitParam(), offsetParam()},
			Responses:   listResponses[commerce.Supplier]("Supplier collection"),
		},
		{
			Method:      http.MethodPost,
			Path:        "/v1/admin/suppliers/{id}/status",
			OperationID: "updateAdminSupplierStatus",
			Summary:     "Update a supplier status",
			Tags:        []string{"Suppliers", "Audit"},
			Auth:        true,
			Parameters:  []ParameterSpec{pathStringParam("id", "Supplier identifier")},
			RequestBody: StatusUpdateRequest{},
			Responses:   authOKResponses("Supplier status updated", StatusResponse{}),
		},
		{
			Method:      http.MethodGet,
			Path:        "/v1/admin/sellers",
			OperationID: "listAdminSellers",
			Summary:     "List sellers",
			Tags:        []string{"Sellers"},
			Auth:        true,
			Parameters:  []ParameterSpec{limitParam(), offsetParam()},
			Responses:   listResponses[commerce.Seller]("Seller collection"),
		},
		{
			Method:      http.MethodPost,
			Path:        "/v1/admin/sellers/{id}/status",
			OperationID: "updateAdminSellerStatus",
			Summary:     "Update a seller status",
			Tags:        []string{"Sellers", "Audit"},
			Auth:        true,
			Parameters:  []ParameterSpec{pathStringParam("id", "Seller identifier")},
			RequestBody: StatusUpdateRequest{},
			Responses:   authOKResponses("Seller status updated", StatusResponse{}),
		},
		{
			Method:      http.MethodGet,
			Path:        "/v1/admin/stores",
			OperationID: "listAdminStores",
			Summary:     "List stores",
			Tags:        []string{"Stores"},
			Auth:        true,
			Parameters:  []ParameterSpec{limitParam(), offsetParam()},
			Responses:   listResponses[commerce.Store]("Store collection"),
		},
		{
			Method:      http.MethodPost,
			Path:        "/v1/admin/stores/{id}/status",
			OperationID: "updateAdminStoreStatus",
			Summary:     "Update a store status",
			Tags:        []string{"Stores", "Audit"},
			Auth:        true,
			Parameters:  []ParameterSpec{pathStringParam("id", "Store identifier")},
			RequestBody: StatusUpdateRequest{},
			Responses:   authOKResponses("Store status updated", StatusResponse{}),
		},
		{
			Method:      http.MethodGet,
			Path:        "/v1/admin/products",
			OperationID: "listAdminProducts",
			Summary:     "List products",
			Tags:        []string{"Catalog", "Attributes", "Variants", "SKUs"},
			Auth:        true,
			Parameters:  []ParameterSpec{limitParam(), offsetParam()},
			Responses:   listResponses[commerce.Product]("Product collection"),
		},
		{
			Method:      http.MethodPost,
			Path:        "/v1/admin/products/{id}/status",
			OperationID: "updateAdminProductStatus",
			Summary:     "Update a product status",
			Tags:        []string{"Catalog", "Audit"},
			Auth:        true,
			Parameters:  []ParameterSpec{pathStringParam("id", "Product identifier")},
			RequestBody: StatusUpdateRequest{},
			Responses:   authOKResponses("Product status updated", StatusResponse{}),
		},
		{
			Method:      http.MethodGet,
			Path:        "/v1/admin/categories",
			OperationID: "listAdminCategories",
			Summary:     "List categories",
			Tags:        []string{"Categories"},
			Auth:        true,
			Parameters:  []ParameterSpec{limitParam(), offsetParam()},
			Responses:   listResponses[commerce.Category]("Category collection"),
		},
		{
			Method:      http.MethodPost,
			Path:        "/v1/admin/categories/{id}/status",
			OperationID: "updateAdminCategoryStatus",
			Summary:     "Update a category status",
			Tags:        []string{"Categories", "Audit"},
			Auth:        true,
			Parameters:  []ParameterSpec{pathStringParam("id", "Category identifier")},
			RequestBody: StatusUpdateRequest{},
			Responses:   authOKResponses("Category status updated", StatusResponse{}),
		},
		{
			Method:      http.MethodGet,
			Path:        "/v1/admin/offers",
			OperationID: "listAdminSupplierOffers",
			Summary:     "List supplier offers",
			Tags:        []string{"Supplier Offers"},
			Auth:        true,
			Parameters:  []ParameterSpec{limitParam(), offsetParam(), stringParam("market_code", "Filter by market code", false)},
			Responses:   listResponses[commerce.SupplierCatalogItem]("Supplier offer collection"),
		},
		{
			Method:      http.MethodPost,
			Path:        "/v1/admin/offers/{id}/status",
			OperationID: "updateAdminOfferStatus",
			Summary:     "Update a supplier offer status",
			Tags:        []string{"Supplier Offers", "Audit"},
			Auth:        true,
			Parameters:  []ParameterSpec{pathStringParam("id", "Supplier offer identifier")},
			RequestBody: StatusUpdateRequest{},
			Responses:   authOKResponses("Supplier offer status updated", StatusResponse{}),
		},
		{
			Method:      http.MethodGet,
			Path:        "/v1/admin/listings",
			OperationID: "listAdminSellerListings",
			Summary:     "List seller listings",
			Tags:        []string{"Seller Listings"},
			Auth:        true,
			Parameters:  []ParameterSpec{limitParam(), offsetParam(), stringParam("store_id", "Filter by store identifier", false)},
			Responses:   listResponses[commerce.SellerListing]("Seller listing collection"),
		},
		{
			Method:      http.MethodPost,
			Path:        "/v1/admin/listings/{id}/status",
			OperationID: "updateAdminListingStatus",
			Summary:     "Update a seller listing status",
			Tags:        []string{"Seller Listings", "Audit"},
			Auth:        true,
			Parameters:  []ParameterSpec{pathStringParam("id", "Seller listing identifier")},
			RequestBody: StatusUpdateRequest{},
			Responses:   authOKResponses("Seller listing status updated", StatusResponse{}),
		},
		{
			Method:      http.MethodGet,
			Path:        "/v1/admin/locations",
			OperationID: "listAdminFulfillmentLocations",
			Summary:     "List fulfillment locations",
			Tags:        []string{"Fulfillment Locations"},
			Auth:        true,
			Parameters:  []ParameterSpec{limitParam(), offsetParam(), stringParam("supplier_id", "Filter by supplier identifier", false)},
			Responses:   listResponses[commerce.FulfillmentLocation]("Fulfillment location collection"),
		},
		{
			Method:      http.MethodPost,
			Path:        "/v1/admin/locations/{id}/status",
			OperationID: "updateAdminLocationStatus",
			Summary:     "Update a fulfillment location status",
			Tags:        []string{"Fulfillment Locations", "Audit"},
			Auth:        true,
			Parameters:  []ParameterSpec{pathStringParam("id", "Fulfillment location identifier")},
			RequestBody: StatusUpdateRequest{},
			Responses:   authOKResponses("Fulfillment location status updated", StatusResponse{}),
		},
	}
}

func supplierRoutes() []RouteSpec {
	return []RouteSpec{
		{
			Method:      http.MethodGet,
			Path:        "/v1/supplier/profile",
			OperationID: "getSupplierProfile",
			Summary:     "Get the supplier profile",
			Tags:        []string{"Suppliers"},
			Auth:        true,
			Responses:   authReadResponses("Supplier profile", SupplierProfileResponse{}),
		},
		{
			Method:      http.MethodPut,
			Path:        "/v1/supplier/profile",
			OperationID: "updateSupplierProfile",
			Summary:     "Update the supplier profile",
			Tags:        []string{"Suppliers"},
			Auth:        true,
			RequestBody: SupplierProfileUpdateRequest{},
			Responses:   authOKResponses("Supplier profile updated", StatusResponse{}),
		},
		{
			Method:      http.MethodGet,
			Path:        "/v1/supplier/markets",
			OperationID: "listSupplierMarkets",
			Summary:     "List supplier markets",
			Tags:        []string{"Markets"},
			Auth:        true,
			Parameters:  []ParameterSpec{limitParam(), offsetParam()},
			Responses:   listResponses[commerce.SupplierMarket]("Supplier market collection"),
		},
		{
			Method:      http.MethodGet,
			Path:        "/v1/supplier/locations",
			OperationID: "listSupplierLocations",
			Summary:     "List fulfillment locations",
			Tags:        []string{"Fulfillment Locations"},
			Auth:        true,
			Parameters:  []ParameterSpec{limitParam(), offsetParam()},
			Responses:   listResponses[commerce.FulfillmentLocation]("Fulfillment location collection"),
		},
		{
			Method:      http.MethodPost,
			Path:        "/v1/supplier/locations",
			OperationID: "createSupplierLocation",
			Summary:     "Create a fulfillment location",
			Tags:        []string{"Fulfillment Locations"},
			Auth:        true,
			RequestBody: SupplierLocationCreateRequest{},
			Responses:   authCreatedResponses("Fulfillment location created", commerce.FulfillmentLocation{}),
		},
		{
			Method:      http.MethodGet,
			Path:        "/v1/supplier/products",
			OperationID: "listSupplierProducts",
			Summary:     "List supplier products",
			Tags:        []string{"Catalog"},
			Auth:        true,
			Parameters:  []ParameterSpec{limitParam(), offsetParam()},
			Responses:   listResponses[commerce.SupplierProduct]("Supplier product collection"),
		},
		{
			Method:      http.MethodPost,
			Path:        "/v1/supplier/products",
			OperationID: "createSupplierProduct",
			Summary:     "Create a supplier product",
			Tags:        []string{"Catalog", "Categories", "Attributes", "Variants", "SKUs"},
			Auth:        true,
			RequestBody: SupplierProductCreateRequest{},
			Responses:   authCreatedResponses("Supplier product created", ProductCreateResponse{}),
		},
		{
			Method:      http.MethodPut,
			Path:        "/v1/supplier/products/{id}/categories",
			OperationID: "setSupplierProductCategories",
			Summary:     "Update supplier product categories",
			Tags:        []string{"Categories"},
			Auth:        true,
			Parameters:  []ParameterSpec{pathStringParam("id", "Product identifier")},
			RequestBody: SupplierProductCategoriesRequest{},
			Responses:   authOKResponses("Supplier product categories updated", SupplierProductCategoriesRequest{}),
		},
		{
			Method:      http.MethodGet,
			Path:        "/v1/supplier/offers",
			OperationID: "listSupplierOffers",
			Summary:     "List supplier offers",
			Tags:        []string{"Supplier Offers"},
			Auth:        true,
			Parameters:  []ParameterSpec{limitParam(), offsetParam()},
			Responses:   listResponses[commerce.SupplierOffer]("Supplier offer collection"),
		},
		{
			Method:      http.MethodPost,
			Path:        "/v1/supplier/offers",
			OperationID: "createSupplierOffer",
			Summary:     "Create a supplier offer",
			Tags:        []string{"Supplier Offers"},
			Auth:        true,
			RequestBody: SupplierOfferCreateRequest{},
			Responses:   authCreatedResponses("Supplier offer created", commerce.SupplierOffer{}),
		},
		{
			Method:      http.MethodGet,
			Path:        "/v1/supplier/inventory",
			OperationID: "listSupplierInventory",
			Summary:     "List inventory snapshots",
			Tags:        []string{"Inventory"},
			Auth:        true,
			Parameters:  []ParameterSpec{limitParam(), offsetParam()},
			Responses:   listResponses[commerce.InventorySnapshot]("Inventory snapshot collection"),
		},
		{
			Method:      http.MethodPost,
			Path:        "/v1/supplier/inventory/snapshots",
			OperationID: "createInventorySnapshot",
			Summary:     "Create an inventory snapshot",
			Tags:        []string{"Inventory"},
			Auth:        true,
			RequestBody: InventorySnapshotCreateRequest{},
			Responses:   authCreatedResponses("Inventory snapshot created", commerce.InventorySnapshot{}),
		},
		{
			Method:      http.MethodPost,
			Path:        "/v1/supplier/inventory/{snapshot_id}/adjustments",
			OperationID: "adjustInventorySnapshot",
			Summary:     "Adjust an inventory snapshot",
			Tags:        []string{"Inventory", "Audit"},
			Auth:        true,
			Parameters:  []ParameterSpec{pathStringParam("snapshot_id", "Inventory snapshot identifier")},
			RequestBody: InventoryAdjustmentRequest{},
			Responses:   authOKResponses("Inventory adjusted", InventoryAdjustmentResponse{}),
		},
		{
			Method:      http.MethodGet,
			Path:        "/v1/supplier/inventory/{snapshot_id}/movements",
			OperationID: "listInventoryMovements",
			Summary:     "List inventory movements",
			Tags:        []string{"Inventory", "Audit"},
			Auth:        true,
			Parameters:  []ParameterSpec{pathStringParam("snapshot_id", "Inventory snapshot identifier"), limitParam(), offsetParam()},
			Responses:   listResponses[commerce.InventoryMovement]("Inventory movement collection"),
		},
	}
}

func sellerRoutes() []RouteSpec {
	return []RouteSpec{
		{
			Method:      http.MethodGet,
			Path:        "/v1/seller/profile",
			OperationID: "getSellerProfile",
			Summary:     "Get the seller profile",
			Tags:        []string{"Sellers"},
			Auth:        true,
			Responses:   authReadResponses("Seller profile", SellerProfileResponse{}),
		},
		{
			Method:      http.MethodPut,
			Path:        "/v1/seller/profile",
			OperationID: "updateSellerProfile",
			Summary:     "Update the seller profile",
			Tags:        []string{"Sellers"},
			Auth:        true,
			RequestBody: SellerProfileUpdateRequest{},
			Responses:   authOKResponses("Seller profile updated", StatusResponse{}),
		},
		{
			Method:      http.MethodGet,
			Path:        "/v1/seller/stores",
			OperationID: "listSellerStores",
			Summary:     "List seller stores",
			Tags:        []string{"Stores"},
			Auth:        true,
			Parameters:  []ParameterSpec{limitParam(), offsetParam()},
			Responses:   listResponses[commerce.Store]("Store collection"),
		},
		{
			Method:      http.MethodPost,
			Path:        "/v1/seller/stores",
			OperationID: "createSellerStore",
			Summary:     "Create a seller store",
			Tags:        []string{"Stores"},
			Auth:        true,
			RequestBody: SellerStoreCreateRequest{},
			Responses:   authCreatedResponses("Store created", commerce.Store{}),
		},
		{
			Method:      http.MethodGet,
			Path:        "/v1/seller/catalog/offers",
			OperationID: "listSellerCatalogOffers",
			Summary:     "List supplier catalog offers",
			Tags:        []string{"Catalog", "Supplier Offers"},
			Auth:        true,
			Parameters: []ParameterSpec{
				stringParam("store_id", "Store identifier", true),
				stringParam("supplier_id", "Filter by supplier identifier", false),
				stringParam("category_id", "Filter by category identifier", false),
				limitParam(),
				offsetParam(),
			},
			Responses: listResponses[commerce.SupplierCatalogItem]("Supplier catalog collection"),
		},
		{
			Method:      http.MethodGet,
			Path:        "/v1/seller/listings",
			OperationID: "listSellerListings",
			Summary:     "List seller listings",
			Tags:        []string{"Seller Listings"},
			Auth:        true,
			Parameters: []ParameterSpec{
				stringParam("store_id", "Store identifier", true),
				limitParam(),
				offsetParam(),
			},
			Responses: listResponses[commerce.SellerListing]("Seller listing collection"),
		},
		{
			Method:      http.MethodPost,
			Path:        "/v1/seller/listings/import",
			OperationID: "importSellerListing",
			Summary:     "Import a supplier offer as a seller listing",
			Tags:        []string{"Seller Listings"},
			Auth:        true,
			RequestBody: SellerListingImportRequest{},
			Responses:   authCreatedResponses("Seller listing imported", commerce.SellerListing{}),
		},
		{
			Method:      http.MethodPost,
			Path:        "/v1/seller/listings/{id}/price",
			OperationID: "updateSellerListingPrice",
			Summary:     "Update seller listing price",
			Tags:        []string{"Seller Listings"},
			Auth:        true,
			Parameters:  []ParameterSpec{pathStringParam("id", "Seller listing identifier")},
			RequestBody: SellerListingPriceRequest{},
			Responses:   authOKResponses("Seller listing price updated", StatusResponse{}),
		},
		{
			Method:      http.MethodPost,
			Path:        "/v1/seller/listings/{id}/status",
			OperationID: "updateSellerListingStatus",
			Summary:     "Update seller listing status",
			Tags:        []string{"Seller Listings", "Audit"},
			Auth:        true,
			Parameters:  []ParameterSpec{pathStringParam("id", "Seller listing identifier")},
			RequestBody: StatusUpdateRequest{},
			Responses:   authOKResponses("Seller listing status updated", StatusResponse{}),
		},
	}
}

func openAPITags() []openapi3.Tag {
	return []openapi3.Tag{
		{Name: "Identity & Access", Description: "Authentication-aware app bootstrap and principal context"},
		{Name: "Markets", Description: "Market bootstrap and discovery"},
		{Name: "Suppliers", Description: "Supplier profile and supplier management"},
		{Name: "Sellers", Description: "Seller profile and seller management"},
		{Name: "Stores", Description: "Seller store management"},
		{Name: "Catalog", Description: "Catalog browsing and product management"},
		{Name: "Categories", Description: "Category management"},
		{Name: "Attributes", Description: "Attribute management"},
		{Name: "Variants", Description: "Variant management"},
		{Name: "SKUs", Description: "SKU management"},
		{Name: "Fulfillment Locations", Description: "Supplier fulfillment locations"},
		{Name: "Supplier Offers", Description: "Supplier offers and availability"},
		{Name: "Seller Listings", Description: "Seller listings and price/status controls"},
		{Name: "Inventory", Description: "Inventory snapshots and movements"},
		{Name: "Audit", Description: "Administrative moderation and operational inspection"},
	}
}

func listResponses[T any](description string) []ResponseSpec {
	return []ResponseSpec{
		okResponse(description, CollectionResponse[T]{}),
		errorResponse(http.StatusUnauthorized, "Unauthorized"),
		errorResponse(http.StatusForbidden, "Forbidden"),
		errorResponse(http.StatusNotFound, "Not found"),
		errorResponse(http.StatusInternalServerError, "Internal error"),
	}
}

func authReadResponses(description string, body any) []ResponseSpec {
	return []ResponseSpec{
		okResponse(description, body),
		errorResponse(http.StatusUnauthorized, "Unauthorized"),
		errorResponse(http.StatusForbidden, "Forbidden"),
		errorResponse(http.StatusNotFound, "Not found"),
		errorResponse(http.StatusInternalServerError, "Internal error"),
	}
}

func authCreatedResponses(description string, body any) []ResponseSpec {
	return []ResponseSpec{
		createdResponse(description, body),
		errorResponse(http.StatusBadRequest, "Validation error"),
		errorResponse(http.StatusUnauthorized, "Unauthorized"),
		errorResponse(http.StatusForbidden, "Forbidden"),
		errorResponse(http.StatusNotFound, "Not found"),
		errorResponse(http.StatusConflict, "Conflict"),
		errorResponse(http.StatusInternalServerError, "Internal error"),
	}
}

func authOKResponses(description string, body any) []ResponseSpec {
	return []ResponseSpec{
		okResponse(description, body),
		errorResponse(http.StatusBadRequest, "Validation error"),
		errorResponse(http.StatusUnauthorized, "Unauthorized"),
		errorResponse(http.StatusForbidden, "Forbidden"),
		errorResponse(http.StatusNotFound, "Not found"),
		errorResponse(http.StatusConflict, "Conflict"),
		errorResponse(http.StatusInternalServerError, "Internal error"),
	}
}

func okResponse(description string, body any) ResponseSpec {
	return ResponseSpec{Status: http.StatusOK, Description: description, Body: body}
}

func createdResponse(description string, body any) ResponseSpec {
	return ResponseSpec{Status: http.StatusCreated, Description: description, Body: body}
}

func errorResponse(status int, description string) ResponseSpec {
	return ResponseSpec{Status: status, Description: description, Body: httpx.ErrorResponse{}}
}

func limitParam() ParameterSpec {
	return ParameterSpec{Name: "limit", In: "query", Required: false, Description: "Page size, capped by the service default", Schema: int64(0)}
}

func offsetParam() ParameterSpec {
	return ParameterSpec{Name: "offset", In: "query", Required: false, Description: "Zero-based offset", Schema: int64(0)}
}

func pathStringParam(name, description string) ParameterSpec {
	return ParameterSpec{Name: name, In: "path", Required: true, Description: description, Schema: ""}
}

func stringParam(name, description string, required bool) ParameterSpec {
	return ParameterSpec{Name: name, In: "query", Required: required, Description: description, Schema: ""}
}
