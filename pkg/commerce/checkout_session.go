package commerce

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const DefaultCheckoutSessionLifetime = 30 * time.Minute

const (
	CheckoutSessionStatusOpen      = "open"
	CheckoutSessionStatusFinalized = "finalized"
	CheckoutSessionStatusExpired   = "expired"
)

type ShippingAddress struct {
	RecipientName string  `json:"recipient_name"`
	AddressLine1  string  `json:"address_line_1"`
	AddressLine2  *string `json:"address_line_2,omitempty"`
	City          string  `json:"city"`
	Region        *string `json:"region,omitempty"`
	PostalCode    *string `json:"postal_code,omitempty"`
	CountryCode   string  `json:"country_code"`
}

type CheckoutSession struct {
	ID                          string          `json:"id"`
	StoreID                     string          `json:"store_id"`
	CartID                      string          `json:"cart_id"`
	CustomerID                  *string         `json:"customer_id,omitempty"`
	Status                      string          `json:"status"`
	ExpiresAt                   time.Time       `json:"expires_at"`
	ShippingAddressSnapshot     json.RawMessage `json:"shipping_address_snapshot,omitempty"`
	ContactEmail                *string         `json:"contact_email,omitempty"`
	FinalizeFingerprint         *string         `json:"-"`
	GuestOrderAccessTokenDigest []byte          `json:"-"`
	FinalizedAt                 *time.Time      `json:"finalized_at,omitempty"`
	CreatedAt                   time.Time       `json:"created_at"`
	UpdatedAt                   time.Time       `json:"updated_at"`
}

type FinalizeRequest struct {
	SessionID       string
	ShippingAddress ShippingAddress
	ContactEmail    string
}

type CheckoutDecision struct {
	SessionID   string
	Status      string
	Fingerprint string
	Replay      bool
}

type fingerprintLine struct {
	SellerListingID        string `json:"seller_listing_id"`
	SKUID                  string `json:"sku_id"`
	Quantity               int64  `json:"quantity"`
	ExpectedUnitPriceMinor int64  `json:"expected_unit_price_minor"`
	ExpectedCurrencyCode   string `json:"expected_currency_code"`
}

type canonicalFinalizeInput struct {
	CheckoutSessionID string            `json:"checkout_session_id"`
	CartID            string            `json:"cart_id"`
	CustomerID        *string           `json:"customer_id"`
	ShippingAddress   ShippingAddress   `json:"shipping_address"`
	ContactEmail      string            `json:"contact_email"`
	CartLines         []fingerprintLine `json:"cart_lines"`
}

// ComputeFinalizeFingerprint serializes a typed, field-ordered semantic
// request. It deliberately does not normalize address/contact values beyond
// the validation contract, so only explicitly equal semantic inputs replay.
func ComputeFinalizeFingerprint(session CheckoutSession, cart Cart, request FinalizeRequest) (string, error) {
	if session.ID == "" || cart.ID == "" || request.SessionID != session.ID || request.ShippingAddress.RecipientName == "" || request.ShippingAddress.AddressLine1 == "" || request.ShippingAddress.City == "" || len(request.ShippingAddress.CountryCode) != 2 || strings.TrimSpace(request.ContactEmail) == "" {
		return "", ErrInvalidInput
	}
	lines := make([]fingerprintLine, 0, len(cart.Items))
	for _, item := range cart.Items {
		if item.SellerListingID == "" || item.SKUID == "" || item.Quantity <= 0 || item.ExpectedUnitPriceMinor < 0 || item.ExpectedCurrencyCode == "" {
			return "", ErrInvalidInput
		}
		lines = append(lines, fingerprintLine{
			SellerListingID: item.SellerListingID, SKUID: item.SKUID, Quantity: item.Quantity,
			ExpectedUnitPriceMinor: item.ExpectedUnitPriceMinor, ExpectedCurrencyCode: item.ExpectedCurrencyCode,
		})
	}
	// Cart loading is deterministic, but sorting here makes the primitive safe
	// for callers that construct a Cart in memory or load it through another store.
	sortFingerprintLines(lines)
	payload, err := json.Marshal(canonicalFinalizeInput{
		CheckoutSessionID: session.ID,
		CartID:            cart.ID,
		CustomerID:        session.CustomerID,
		ShippingAddress:   request.ShippingAddress,
		ContactEmail:      request.ContactEmail,
		CartLines:         lines,
	})
	if err != nil {
		return "", fmt.Errorf("canonicalize finalize request: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func sortFingerprintLines(lines []fingerprintLine) {
	for i := 1; i < len(lines); i++ {
		for j := i; j > 0 && (lines[j].SellerListingID < lines[j-1].SellerListingID || (lines[j].SellerListingID == lines[j-1].SellerListingID && lines[j].SKUID < lines[j-1].SKUID)); j-- {
			lines[j], lines[j-1] = lines[j-1], lines[j]
		}
	}
}

func (r Repository) CreateCheckoutSession(ctx context.Context, storeID, cartToken string, customerID *string, lifetime time.Duration) (CheckoutSession, string, error) {
	if storeID == "" || strings.TrimSpace(cartToken) == "" {
		return CheckoutSession{}, "", ErrInvalidInput
	}
	if lifetime == 0 {
		lifetime = DefaultCheckoutSessionLifetime
	}
	if lifetime < 0 || lifetime < time.Second {
		return CheckoutSession{}, "", ErrInvalidInput
	}
	rawCapability, digestHex, err := generateCapability()
	if err != nil {
		return CheckoutSession{}, "", err
	}
	digest, err := hex.DecodeString(digestHex)
	if err != nil {
		return CheckoutSession{}, "", fmt.Errorf("decode capability digest: %w", err)
	}
	var session CheckoutSession
	err = r.withTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		cart, err := r.getCart(ctx, tx, storeID, bearerDigest(cartToken))
		if err != nil {
			return err
		}
		if cart.Status != CartStatusActive {
			return ErrCartExpired
		}
		if customerID != nil && (cart.CustomerID == nil || *customerID != *cart.CustomerID) {
			return ErrConflict
		}
		id := uuid.NewString()
		err = tx.QueryRow(ctx, `
			INSERT INTO checkout_sessions (id, store_id, cart_id, customer_id, status, expires_at, guest_order_access_token_digest)
			VALUES ($1, $2, $3, $4, 'open', clock_timestamp() + ($5 * interval '1 second'), $6)
			RETURNING id, store_id, cart_id, customer_id, status, expires_at, shipping_address_snapshot, contact_email, finalize_fingerprint, guest_order_access_token_digest, finalized_at, created_at, updated_at
		`, id, storeID, cart.ID, customerID, int64(lifetime/time.Second), digest).Scan(checkoutSessionScanArgs(&session)...)
		return translatePGError(err, "create checkout session")
	})
	if err != nil {
		return CheckoutSession{}, "", err
	}
	return session, rawCapability, nil
}

func checkoutSessionScanArgs(session *CheckoutSession) []any {
	return []any{&session.ID, &session.StoreID, &session.CartID, &session.CustomerID, &session.Status, &session.ExpiresAt, &session.ShippingAddressSnapshot, &session.ContactEmail, &session.FinalizeFingerprint, &session.GuestOrderAccessTokenDigest, &session.FinalizedAt, &session.CreatedAt, &session.UpdatedAt}
}

func (r Repository) EvaluateCheckoutSession(ctx context.Context, storeID string, request FinalizeRequest) (CheckoutDecision, error) {
	if storeID == "" || request.SessionID == "" {
		return CheckoutDecision{}, ErrInvalidInput
	}
	var decision CheckoutDecision
	err := r.withTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var session CheckoutSession
		if err := tx.QueryRow(ctx, `
			SELECT id, store_id, cart_id, customer_id, status, expires_at, shipping_address_snapshot, contact_email, finalize_fingerprint, guest_order_access_token_digest, finalized_at, created_at, updated_at
			FROM checkout_sessions WHERE id = $1 AND store_id = $2 FOR UPDATE
		`, request.SessionID, storeID).Scan(checkoutSessionScanArgs(&session)...); err != nil {
			return translatePGError(err, "lock checkout session")
		}
		var cart Cart
		if err := tx.QueryRow(ctx, `
			SELECT id, store_id, market_code, customer_id, status, expires_at, created_at, updated_at
			FROM carts WHERE id = $1 AND store_id = $2 FOR UPDATE
		`, session.CartID, session.StoreID).Scan(&cart.ID, &cart.StoreID, &cart.MarketCode, &cart.CustomerID, &cart.Status, &cart.ExpiresAt, &cart.CreatedAt, &cart.UpdatedAt); err != nil {
			return translatePGError(err, "lock checkout cart")
		}
		var decisionNow time.Time
		if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&decisionNow); err != nil {
			return fmt.Errorf("capture checkout decision time: %w", err)
		}
		if session.Status == CheckoutSessionStatusFinalized && cart.Status != CartStatusCheckedOut {
			return ErrCheckoutCartInvariant
		}
		if session.Status == CheckoutSessionStatusOpen && cart.Status != CartStatusActive {
			return ErrConflict
		}
		if session.Status == CheckoutSessionStatusExpired || (session.Status == CheckoutSessionStatusOpen && !session.ExpiresAt.After(decisionNow)) {
			return ErrCheckoutExpired
		}
		if session.Status != CheckoutSessionStatusOpen && session.Status != CheckoutSessionStatusFinalized {
			return ErrCheckoutExpired
		}
		if len(session.GuestOrderAccessTokenDigest) != sha256.Size {
			return ErrInvalidInput
		}
		if err := loadCartItems(ctx, tx, &cart); err != nil {
			return err
		}
		fingerprint, err := ComputeFinalizeFingerprint(session, cart, request)
		if err != nil {
			return err
		}
		decision = CheckoutDecision{SessionID: session.ID, Status: session.Status, Fingerprint: fingerprint}
		if session.Status == CheckoutSessionStatusFinalized {
			if session.FinalizeFingerprint == nil || *session.FinalizeFingerprint != fingerprint {
				return ErrIdempotencyConflict
			}
			decision.Replay = true
		}
		return nil
	})
	return decision, err
}
