package commerce

import (
	"time"

	"github.com/matjeroapps/core/packages/money"
)

type Supplier struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SupplierSettings struct {
	SupplierID string         `json:"supplier_id"`
	Settings   map[string]any `json:"settings"`
}

type SupplierMember struct {
	ID               string    `json:"id"`
	SupplierID       string    `json:"supplier_id"`
	PrincipalSubject string    `json:"principal_subject"`
	Role             string    `json:"role"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type SupplierMarket struct {
	ID         string         `json:"id"`
	SupplierID string         `json:"supplier_id"`
	MarketCode string         `json:"market_code"`
	Status     string         `json:"status"`
	Settings   map[string]any `json:"settings"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

type Seller struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SellerSettings struct {
	SellerID string         `json:"seller_id"`
	Settings map[string]any `json:"settings"`
}

type SellerMember struct {
	ID               string    `json:"id"`
	SellerID         string    `json:"seller_id"`
	PrincipalSubject string    `json:"principal_subject"`
	Role             string    `json:"role"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type SupplierSellerAffiliation struct {
	SupplierID string    `json:"supplier_id"`
	SellerID   string    `json:"seller_id"`
	CreatedAt  time.Time `json:"created_at"`
}

type RetailCapabilityDraft struct {
	Code     string         `json:"code"`
	Name     string         `json:"name"`
	Settings map[string]any `json:"settings"`
}

type Store struct {
	ID         string    `json:"id"`
	SellerID   string    `json:"seller_id"`
	MarketCode string    `json:"market_code"`
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type StoreDomain struct {
	ID                string     `json:"id"`
	StoreID           string     `json:"store_id"`
	Domain            string     `json:"domain"`
	IsPrimary         bool       `json:"is_primary"`
	VerifiedAt        *time.Time `json:"verified_at,omitempty"`
	Status            string     `json:"status"`
	DomainType        string     `json:"domain_type,omitempty"`
	VerificationToken *string    `json:"verification_token,omitempty"`
	LastCheckedAt     *time.Time `json:"last_checked_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type StoreDomainResponse struct {
	ID                string                   `json:"id"`
	StoreID           string                   `json:"store_id"`
	Domain            string                   `json:"domain"`
	IsPrimary         bool                     `json:"is_primary"`
	VerifiedAt        *time.Time               `json:"verified_at,omitempty"`
	Status            string                   `json:"status"`
	DomainType        string                   `json:"domain_type,omitempty"`
	VerificationToken *string                  `json:"verification_token,omitempty"`
	LastCheckedAt     *time.Time               `json:"last_checked_at,omitempty"`
	Verification      *StoreDomainVerification `json:"verification,omitempty"`
	CreatedAt         time.Time                `json:"created_at"`
	UpdatedAt         time.Time                `json:"updated_at"`
}

func (d StoreDomain) ToResponse() StoreDomainResponse {
	resp := StoreDomainResponse{
		ID:                d.ID,
		StoreID:           d.StoreID,
		Domain:            d.Domain,
		IsPrimary:         d.IsPrimary,
		VerifiedAt:        d.VerifiedAt,
		Status:            d.Status,
		DomainType:        d.DomainType,
		VerificationToken: d.VerificationToken,
		LastCheckedAt:     d.LastCheckedAt,
		CreatedAt:         d.CreatedAt,
		UpdatedAt:         d.UpdatedAt,
	}
	if d.VerificationToken != nil && *d.VerificationToken != "" {
		v := BuildVerificationDetails(d.Domain, *d.VerificationToken)
		resp.Verification = &v
	}
	return resp
}

type StoreDomainAdminResponse struct {
	ID            string     `json:"id"`
	StoreID       string     `json:"store_id"`
	Domain        string     `json:"domain"`
	IsPrimary     bool       `json:"is_primary"`
	VerifiedAt    *time.Time `json:"verified_at,omitempty"`
	Status        string     `json:"status"`
	DomainType    string     `json:"domain_type,omitempty"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (d StoreDomain) ToAdminResponse() StoreDomainAdminResponse {
	return StoreDomainAdminResponse{
		ID:            d.ID,
		StoreID:       d.StoreID,
		Domain:        d.Domain,
		IsPrimary:     d.IsPrimary,
		VerifiedAt:    d.VerifiedAt,
		Status:        d.Status,
		DomainType:    d.DomainType,
		LastCheckedAt: d.LastCheckedAt,
		CreatedAt:     d.CreatedAt,
		UpdatedAt:     d.UpdatedAt,
	}
}

type AdminDomainFilter struct {
	StoreID    string
	SellerID   string
	Status     string
	DomainType string
	Search     string
	Page       Page
}

type StoreSettings struct {
	StoreID  string         `json:"store_id"`
	Settings map[string]any `json:"settings"`
}

type Product struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ProductTranslation struct {
	ProductID   string `json:"product_id"`
	Locale      string `json:"locale"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Category struct {
	ID               string    `json:"id"`
	ParentCategoryID *string   `json:"parent_category_id,omitempty"`
	Slug             string    `json:"slug"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type CategoryTranslation struct {
	CategoryID  string `json:"category_id"`
	Locale      string `json:"locale"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Variant struct {
	ID        string    `json:"id"`
	ProductID string    `json:"product_id"`
	Code      string    `json:"code"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SKU struct {
	ID        string    `json:"id"`
	VariantID string    `json:"variant_id"`
	Code      string    `json:"code"`
	Barcode   string    `json:"barcode,omitempty"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Attribute struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AttributeTranslation struct {
	AttributeID string `json:"attribute_id"`
	Locale      string `json:"locale"`
	Name        string `json:"name"`
}

type AttributeValue struct {
	ID          string    `json:"id"`
	AttributeID string    `json:"attribute_id"`
	Code        string    `json:"code"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AttributeValueTranslation struct {
	AttributeValueID string `json:"attribute_value_id"`
	Locale           string `json:"locale"`
	Name             string `json:"name"`
}

type MediaMetadata struct {
	ID        string         `json:"id"`
	ProductID string         `json:"product_id"`
	MediaType string         `json:"media_type"`
	URI       string         `json:"uri"`
	AltText   string         `json:"alt_text"`
	SortOrder int            `json:"sort_order"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type SupplierProduct struct {
	ID           string    `json:"id"`
	SupplierID   string    `json:"supplier_id"`
	ProductID    string    `json:"product_id"`
	SupplierCode string    `json:"supplier_code"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type SupplierOffer struct {
	ID                string    `json:"id"`
	SupplierID        string    `json:"supplier_id"`
	SupplierProductID string    `json:"supplier_product_id"`
	SupplierMarketID  string    `json:"supplier_market_id"`
	MarketCode        string    `json:"market_code"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type SupplierOfferPrice struct {
	ID              string      `json:"id"`
	SupplierOfferID string      `json:"supplier_offer_id"`
	Price           money.Money `json:"price"`
	IsCurrent       bool        `json:"is_current"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

type SupplierOfferAvailability struct {
	ID              string    `json:"id"`
	SupplierOfferID string    `json:"supplier_offer_id"`
	IsAvailable     bool      `json:"is_available"`
	AvailableQty    *int64    `json:"available_qty,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type SellerListing struct {
	ID              string    `json:"id"`
	StoreID         string    `json:"store_id"`
	ProductID       string    `json:"product_id"`
	SupplierOfferID *string   `json:"supplier_offer_id,omitempty"`
	MarketCode      string    `json:"market_code"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type SellerListingPrice struct {
	ID              string      `json:"id"`
	SellerListingID string      `json:"seller_listing_id"`
	Price           money.Money `json:"price"`
	IsCurrent       bool        `json:"is_current"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

type FulfillmentLocation struct {
	SupplierID       string    `json:"supplier_id"`
	ID               string    `json:"id"`
	SupplierMarketID string    `json:"supplier_market_id"`
	MarketCode       string    `json:"market_code"`
	Code             string    `json:"code"`
	Name             string    `json:"name"`
	LocationType     string    `json:"location_type"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type InventorySnapshot struct {
	ID                    string    `json:"id"`
	FulfillmentLocationID string    `json:"fulfillment_location_id"`
	SKUID                 string    `json:"sku_id"`
	OnHandQty             int64     `json:"on_hand_qty"`
	ReservedQty           int64     `json:"reserved_qty"`
	Version               int64     `json:"version"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type InventoryReservation struct {
	ID                  string     `json:"id"`
	InventorySnapshotID string     `json:"inventory_snapshot_id"`
	Quantity            int64      `json:"quantity"`
	Status              string     `json:"status"`
	ReservationToken    string     `json:"reservation_token"`
	ExpiresAt           *time.Time `json:"expires_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}
