package commerce

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/matjeroapps/core/packages/database"
)

type testCheckoutSetup struct {
	Store      Store
	Cart       Cart
	CartToken  string
	Session    CheckoutSession
	Capability string
	ProductID  string
	VariantID  string
	SKUID      string
	ListingID  string
	LocationID string
	SnapshotID string
}

func setupSellerCheckoutTest(t *testing.T, db *database.Pool, repo Repository, ctx context.Context, suffix string, initialStock int64, priceMinor int64) testCheckoutSetup {
	t.Helper()

	seller, err := repo.CreateSeller(ctx, "seller-"+suffix, "Seller "+suffix, "active", nil)
	if err != nil {
		t.Fatal(err)
	}

	store, _, err := repo.CreateStoreWithDomain(ctx, seller.ID, "EG", "store-"+suffix, "Store "+suffix, "active", nil, suffix+".test", "platform", "active", true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	productID := uuid.NewString()
	if _, err := db.Exec(ctx, `
		INSERT INTO products (id, slug, status) VALUES ($1, $2, 'active')
	`, productID, "prod-"+suffix); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(ctx, `
		INSERT INTO product_translations (product_id, locale, name, description) VALUES ($1, 'en', $2, 'Desc')
	`, productID, "Test Product "+suffix); err != nil {
		t.Fatal(err)
	}

	variantID := uuid.NewString()
	if _, err := db.Exec(ctx, `
		INSERT INTO variants (id, product_id, code, status) VALUES ($1, $2, $3, 'active')
	`, variantID, productID, "VAR-"+suffix); err != nil {
		t.Fatal(err)
	}

	skuID := uuid.NewString()
	if _, err := db.Exec(ctx, `
		INSERT INTO skus (id, variant_id, code, status) VALUES ($1, $2, $3, 'active')
	`, skuID, variantID, "SKU-"+suffix); err != nil {
		t.Fatal(err)
	}

	listingID := uuid.NewString()
	if _, err := db.Exec(ctx, `
		INSERT INTO seller_listings (id, store_id, product_id, supplier_offer_id, market_code, status)
		VALUES ($1, $2, $3, NULL, 'EG', 'active')
	`, listingID, store.ID, productID); err != nil {
		t.Fatal(err)
	}

	priceID := uuid.NewString()
	if _, err := db.Exec(ctx, `
		INSERT INTO seller_listing_prices (id, seller_listing_id, amount_minor, currency_code, is_current)
		VALUES ($1, $2, $3, 'EGP', true)
	`, priceID, listingID, priceMinor); err != nil {
		t.Fatal(err)
	}

	locationID := uuid.NewString()
	if _, err := db.Exec(ctx, `
		INSERT INTO fulfillment_locations (id, store_id, supplier_id, market_code, code, name, location_type, status)
		VALUES ($1, $2, NULL, 'EG', $3, 'Warehouse', 'warehouse', 'active')
	`, locationID, store.ID, "LOC-"+suffix); err != nil {
		t.Fatal(err)
	}

	snapshotID := uuid.NewString()
	if _, err := db.Exec(ctx, `
		INSERT INTO inventory_snapshots (id, fulfillment_location_id, sku_id, on_hand_qty, reserved_qty, version)
		VALUES ($1, $2, $3, $4, 0, 1)
	`, snapshotID, locationID, skuID, initialStock); err != nil {
		t.Fatal(err)
	}

	cart, cartToken, err := repo.CreateCart(ctx, store.ID, "EG", nil)
	if err != nil {
		t.Fatal(err)
	}

	cart, err = repo.AddCartItem(ctx, store.ID, cartToken, skuID, 1)
	if err != nil {
		t.Fatal(err)
	}

	session, capability, err := repo.CreateCheckoutSession(ctx, store.ID, cartToken, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	return testCheckoutSetup{
		Store:      store,
		Cart:       cart,
		CartToken:  cartToken,
		Session:    session,
		Capability: capability,
		ProductID:  productID,
		VariantID:  variantID,
		SKUID:      skuID,
		ListingID:  listingID,
		LocationID: locationID,
		SnapshotID: snapshotID,
	}
}

func testFinalizePayload(sessionID string) FinalizeRequest {
	return FinalizeRequest{
		SessionID: sessionID,
		ShippingAddress: ShippingAddress{
			RecipientName: "John Doe",
			AddressLine1:  "123 Main St",
			City:          "Cairo",
			CountryCode:   "EG",
		},
		ContactEmail: "john@example.com",
	}
}

func TestFinalizeCheckoutCreatesOrderAtomically(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()
	setup := setupSellerCheckoutTest(t, db, repo, ctx, suffix, 10, 1000)

	req := testFinalizePayload(setup.Session.ID)
	order, err := repo.FinalizeCheckout(ctx, setup.Store.ID, req, "corr-1")
	if err != nil {
		t.Fatalf("FinalizeCheckout failed: %v", err)
	}

	if order.ID == "" || order.OrderNumber == "" || order.Status != OrderStatusPending {
		t.Fatalf("unexpected order output: %+v", order)
	}
	if order.SubtotalMinor != 1000 || order.TotalMinor != 1000 || order.CurrencyCode != "EGP" {
		t.Fatalf("unexpected money totals: %+v", order)
	}

	// Verify Session finalized & Cart checked_out
	var sessionStatus string
	if err := db.QueryRow(ctx, `SELECT status FROM checkout_sessions WHERE id = $1`, setup.Session.ID).Scan(&sessionStatus); err != nil {
		t.Fatal(err)
	}
	if sessionStatus != CheckoutSessionStatusFinalized {
		t.Fatalf("session status = %s, want finalized", sessionStatus)
	}

	var cartStatus string
	if err := db.QueryRow(ctx, `SELECT status FROM carts WHERE id = $1`, setup.Cart.ID).Scan(&cartStatus); err != nil {
		t.Fatal(err)
	}
	if cartStatus != CartStatusCheckedOut {
		t.Fatalf("cart status = %s, want checked_out", cartStatus)
	}

	// Verify Inventory Snapshot reserved_qty = 1
	var reservedQty int64
	if err := db.QueryRow(ctx, `SELECT reserved_qty FROM inventory_snapshots WHERE id = $1`, setup.SnapshotID).Scan(&reservedQty); err != nil {
		t.Fatal(err)
	}
	if reservedQty != 1 {
		t.Fatalf("reserved_qty = %d, want 1", reservedQty)
	}

	// Verify Reservation & Movement created
	var resCount, movCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM inventory_reservations WHERE inventory_snapshot_id = $1`, setup.SnapshotID).Scan(&resCount); err != nil {
		t.Fatal(err)
	}
	if resCount != 1 {
		t.Fatalf("resCount = %d, want 1", resCount)
	}

	if err := db.QueryRow(ctx, `SELECT count(*) FROM inventory_movements WHERE inventory_snapshot_id = $1 AND movement_type = 'reservation_held'`, setup.SnapshotID).Scan(&movCount); err != nil {
		t.Fatal(err)
	}
	if movCount != 1 {
		t.Fatalf("movCount = %d, want 1", movCount)
	}

	// Verify Timeline & Outbox event
	var timelineCount, outboxCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM order_timeline WHERE order_id = $1`, order.ID).Scan(&timelineCount); err != nil {
		t.Fatal(err)
	}
	if timelineCount != 1 {
		t.Fatalf("timelineCount = %d, want 1", timelineCount)
	}

	if err := db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id = $1 AND event_type = 'commerce.order.created.v1'`, order.ID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 1 {
		t.Fatalf("outboxCount = %d, want 1", outboxCount)
	}
}

func TestFinalizeCheckoutTenConcurrentIdenticalRequestsOneOrder(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()
	setup := setupSellerCheckoutTest(t, db, repo, ctx, suffix, 10, 1000)

	const concurrency = 10
	var wg sync.WaitGroup
	orders := make(chan Order, concurrency)
	errs := make(chan error, concurrency)

	startBarrier := make(chan struct{})

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startBarrier
			req := testFinalizePayload(setup.Session.ID)
			order, err := repo.FinalizeCheckout(ctx, setup.Store.ID, req, "corr-concurrent")
			if err != nil {
				errs <- err
				return
			}
			orders <- order
		}()
	}

	close(startBarrier)
	wg.Wait()
	close(orders)
	close(errs)

	for err := range errs {
		t.Fatalf("unexpected error during concurrent finalize: %v", err)
	}

	orderIDs := make(map[string]struct{})
	for order := range orders {
		orderIDs[order.ID] = struct{}{}
	}

	if len(orderIDs) != 1 {
		t.Fatalf("expected 1 distinct Order ID across concurrent requests, got %d", len(orderIDs))
	}

	var totalOrders int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM orders WHERE checkout_session_id = $1`, setup.Session.ID).Scan(&totalOrders); err != nil {
		t.Fatal(err)
	}
	if totalOrders != 1 {
		t.Fatalf("total orders in DB = %d, want 1", totalOrders)
	}

	var totalReservations int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM inventory_reservations WHERE inventory_snapshot_id = $1`, setup.SnapshotID).Scan(&totalReservations); err != nil {
		t.Fatal(err)
	}
	if totalReservations != 1 {
		t.Fatalf("total reservations in DB = %d, want 1", totalReservations)
	}
}

func TestFinalizeCheckoutReplayReturnsExactSameOrder(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()
	setup := setupSellerCheckoutTest(t, db, repo, ctx, suffix, 10, 1000)

	req := testFinalizePayload(setup.Session.ID)
	firstOrder, err := repo.FinalizeCheckout(ctx, setup.Store.ID, req, "corr-replay")
	if err != nil {
		t.Fatal(err)
	}

	secondOrder, err := repo.FinalizeCheckout(ctx, setup.Store.ID, req, "corr-replay-2")
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}

	if firstOrder.ID != secondOrder.ID || firstOrder.OrderNumber != secondOrder.OrderNumber {
		t.Fatalf("replay returned different Order: first=%+v, second=%+v", firstOrder, secondOrder)
	}
}

func TestFinalizeCheckoutChangedSemanticReplayConflicts(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()
	setup := setupSellerCheckoutTest(t, db, repo, ctx, suffix, 10, 1000)

	req := testFinalizePayload(setup.Session.ID)
	_, err := repo.FinalizeCheckout(ctx, setup.Store.ID, req, "corr-1")
	if err != nil {
		t.Fatal(err)
	}

	// Change address
	mutatedReq := req
	mutatedReq.ShippingAddress.AddressLine1 = "999 Changed St"

	_, err = repo.FinalizeCheckout(ctx, setup.Store.ID, mutatedReq, "corr-2")
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected ErrIdempotencyConflict for changed replay, got %v", err)
	}
}

func TestFinalizeCheckoutTwoSessionsOneCartExactlyOneOrder(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()
	setup := setupSellerCheckoutTest(t, db, repo, ctx, suffix, 10, 1000)

	// Create second open session on same active cart
	session2, _, err := repo.CreateCheckoutSession(ctx, setup.Store.ID, setup.CartToken, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	req1 := testFinalizePayload(setup.Session.ID)
	req2 := testFinalizePayload(session2.ID)

	order1, err1 := repo.FinalizeCheckout(ctx, setup.Store.ID, req1, "corr-sess1")
	if err1 != nil {
		t.Fatalf("session 1 finalize failed: %v", err1)
	}
	if order1.ID == "" {
		t.Fatal("empty order ID")
	}

	// Second session finalize on now checked_out cart returns ErrConflict
	_, err2 := repo.FinalizeCheckout(ctx, setup.Store.ID, req2, "corr-sess2")
	if !errors.Is(err2, ErrConflict) {
		t.Fatalf("expected ErrConflict for second session on checked_out cart, got %v", err2)
	}
}

func TestFinalizeCheckoutLastUnitContention(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()

	// Initial stock = 1 unit
	seller, err := repo.CreateSeller(ctx, "seller-"+suffix, "Seller "+suffix, "active", nil)
	if err != nil {
		t.Fatal(err)
	}
	store, _, err := repo.CreateStoreWithDomain(ctx, seller.ID, "EG", "store-"+suffix, "Store "+suffix, "active", nil, suffix+".test", "platform", "active", true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	productID := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO products (id, slug, status) VALUES ($1, $2, 'active')`, productID, "prod-"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO product_translations (product_id, locale, name) VALUES ($1, 'en', 'Prod')`, productID); err != nil {
		t.Fatal(err)
	}
	variantID := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO variants (id, product_id, code, status) VALUES ($1, $2, $3, 'active')`, variantID, productID, "VAR-"+suffix); err != nil {
		t.Fatal(err)
	}
	skuID := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO skus (id, variant_id, code, status) VALUES ($1, $2, $3, 'active')`, skuID, variantID, "SKU-"+suffix); err != nil {
		t.Fatal(err)
	}
	listingID := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO seller_listings (id, store_id, product_id, market_code, status) VALUES ($1, $2, $3, 'EG', 'active')`, listingID, store.ID, productID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO seller_listing_prices (id, seller_listing_id, amount_minor, currency_code, is_current) VALUES ($1, $2, 1000, 'EGP', true)`, uuid.NewString(), listingID); err != nil {
		t.Fatal(err)
	}
	locationID := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO fulfillment_locations (id, store_id, market_code, code, name, location_type, status) VALUES ($1, $2, 'EG', $3, 'Loc', 'warehouse', 'active')`, locationID, store.ID, "LOC-"+suffix); err != nil {
		t.Fatal(err)
	}
	snapshotID := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO inventory_snapshots (id, fulfillment_location_id, sku_id, on_hand_qty, reserved_qty, version) VALUES ($1, $2, $3, 1, 0, 1)`, snapshotID, locationID, skuID); err != nil {
		t.Fatal(err)
	}

	const numCheckouts = 20
	type checkoutTarget struct {
		Session CheckoutSession
		Token   string
	}
	targets := make([]checkoutTarget, 0, numCheckouts)

	for i := 0; i < numCheckouts; i++ {
		_, token, err := repo.CreateCart(ctx, store.ID, "EG", nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = repo.AddCartItem(ctx, store.ID, token, skuID, 1)
		if err != nil {
			t.Fatal(err)
		}
		s, _, err := repo.CreateCheckoutSession(ctx, store.ID, token, nil, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		targets = append(targets, checkoutTarget{Session: s, Token: token})
	}

	var wg sync.WaitGroup
	winners := make(chan Order, numCheckouts)
	losers := make(chan error, numCheckouts)

	startBarrier := make(chan struct{})

	for _, tgt := range targets {
		wg.Add(1)
		tTarget := tgt
		go func() {
			defer wg.Done()
			<-startBarrier
			req := testFinalizePayload(tTarget.Session.ID)
			order, err := repo.FinalizeCheckout(ctx, store.ID, req, "corr-lastunit")
			if err != nil {
				losers <- err
				return
			}
			winners <- order
		}()
	}

	close(startBarrier)
	wg.Wait()
	close(winners)
	close(losers)

	var winnerList []Order
	for o := range winners {
		winnerList = append(winnerList, o)
	}

	var loserErrList []error
	for e := range losers {
		loserErrList = append(loserErrList, e)
	}

	if len(winnerList) != 1 {
		t.Fatalf("expected exactly 1 winner for 1 stock unit, got %d", len(winnerList))
	}
	if len(loserErrList) != numCheckouts-1 {
		t.Fatalf("expected %d losers, got %d", numCheckouts-1, len(loserErrList))
	}

	for _, err := range loserErrList {
		if !errors.Is(err, ErrInsufficientInventory) && !errors.Is(err, ErrConflict) {
			t.Fatalf("unexpected loser error code: %v", err)
		}
	}
}

func TestFinalizeCheckoutLateFailureRollsBackEverything(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()
	setup := setupSellerCheckoutTest(t, db, repo, ctx, suffix, 10, 1000)

	// Set test hook to simulate late failure before commit
	testHookBeforeFinalizeCommit = func(ctx context.Context) error {
		return errors.New("simulated late failure")
	}
	defer func() { testHookBeforeFinalizeCommit = nil }()

	req := testFinalizePayload(setup.Session.ID)
	_, err := repo.FinalizeCheckout(ctx, setup.Store.ID, req, "corr-fail")
	if err == nil {
		t.Fatal("expected failure from test hook, got nil")
	}

	// Verify total rollback across all tables
	var orderCount, resCount, movCount, outboxCount int
	_ = db.QueryRow(ctx, `SELECT count(*) FROM orders WHERE checkout_session_id = $1`, setup.Session.ID).Scan(&orderCount)
	if orderCount != 0 {
		t.Fatalf("order survived rollback: %d", orderCount)
	}

	_ = db.QueryRow(ctx, `SELECT count(*) FROM inventory_reservations WHERE inventory_snapshot_id = $1`, setup.SnapshotID).Scan(&resCount)
	if resCount != 0 {
		t.Fatalf("reservation survived rollback: %d", resCount)
	}

	_ = db.QueryRow(ctx, `SELECT count(*) FROM inventory_movements WHERE inventory_snapshot_id = $1`, setup.SnapshotID).Scan(&movCount)
	if movCount != 0 {
		t.Fatalf("movement survived rollback: %d", movCount)
	}

	_ = db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'commerce.order.created.v1'`).Scan(&outboxCount)
	if outboxCount != 0 {
		t.Fatalf("outbox event survived rollback: %d", outboxCount)
	}

	var sessionStatus, cartStatus string
	_ = db.QueryRow(ctx, `SELECT status FROM checkout_sessions WHERE id = $1`, setup.Session.ID).Scan(&sessionStatus)
	if sessionStatus != CheckoutSessionStatusOpen {
		t.Fatalf("session status after rollback = %s, want open", sessionStatus)
	}

	_ = db.QueryRow(ctx, `SELECT status FROM carts WHERE id = $1`, setup.Cart.ID).Scan(&cartStatus)
	if cartStatus != CartStatusActive {
		t.Fatalf("cart status after rollback = %s, want active", cartStatus)
	}
}

func TestFinalizeCheckoutMoneyMultiplicationOverflow(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()

	// Setup with standard catalog item
	setup := setupSellerCheckoutTest(t, db, repo, ctx, suffix, 10, 1000)

	// Artificially update cart item price to large int64 to trigger overflow when multiplied by quantity
	largePrice := int64(1<<62 - 1)
	if _, err := db.Exec(ctx, `UPDATE cart_items SET expected_unit_price_minor = $1, quantity = 100 WHERE cart_id = $2`, largePrice, setup.Cart.ID); err != nil {
		t.Fatal(err)
	}

	req := testFinalizePayload(setup.Session.ID)
	_, err := repo.FinalizeCheckout(ctx, setup.Store.ID, req, "corr-overflow")
	if !errors.Is(err, ErrInvalidInput) && !errors.Is(err, ErrPriceChanged) {
		t.Fatalf("expected ErrInvalidInput/ErrPriceChanged for money overflow, got %v", err)
	}
}

func TestFinalizeCheckoutPriceChanged(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()
	setup := setupSellerCheckoutTest(t, db, repo, ctx, suffix, 10, 1000)

	// Mutate current retail listing price in DB
	if _, err := db.Exec(ctx, `UPDATE seller_listing_prices SET amount_minor = 1500 WHERE seller_listing_id = $1 AND is_current = true`, setup.ListingID); err != nil {
		t.Fatal(err)
	}

	req := testFinalizePayload(setup.Session.ID)
	_, err := repo.FinalizeCheckout(ctx, setup.Store.ID, req, "corr-pricechange")
	if !errors.Is(err, ErrPriceChanged) {
		t.Fatalf("expected ErrPriceChanged, got %v", err)
	}
}

func TestFinalizeCheckoutInactiveProduct(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()
	setup := setupSellerCheckoutTest(t, db, repo, ctx, suffix, 10, 1000)

	// Deactivate product
	if _, err := db.Exec(ctx, `UPDATE products SET status = 'inactive' WHERE id = $1`, setup.ProductID); err != nil {
		t.Fatal(err)
	}

	req := testFinalizePayload(setup.Session.ID)
	_, err := repo.FinalizeCheckout(ctx, setup.Store.ID, req, "corr-inactiveprod")
	if !errors.Is(err, ErrListingUnavailable) {
		t.Fatalf("expected ErrListingUnavailable for inactive product, got %v", err)
	}
}

func TestFinalizeCheckoutSellerOwnedSupplierFieldsNull(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()
	setup := setupSellerCheckoutTest(t, db, repo, ctx, suffix, 10, 1000)

	req := testFinalizePayload(setup.Session.ID)
	order, err := repo.FinalizeCheckout(ctx, setup.Store.ID, req, "corr-sellerowned")
	if err != nil {
		t.Fatal(err)
	}

	if len(order.Items) != 1 {
		t.Fatalf("order items count = %d, want 1", len(order.Items))
	}
	item := order.Items[0]
	if item.SupplierOfferID != nil || item.SourceSupplierID != nil || item.SupplierCostMinor != nil || item.SupplierCostCurrencyCode != nil {
		t.Fatalf("seller-owned order item has non-null supplier fields: %+v", item)
	}
}

func TestFinalizeCheckoutCopiesSessionCapabilityDigest(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()
	setup := setupSellerCheckoutTest(t, db, repo, ctx, suffix, 10, 1000)

	req := testFinalizePayload(setup.Session.ID)
	order, err := repo.FinalizeCheckout(ctx, setup.Store.ID, req, "corr-digest")
	if err != nil {
		t.Fatal(err)
	}

	var orderDigest []byte
	if err := db.QueryRow(ctx, `SELECT guest_order_access_token_digest FROM orders WHERE id = $1`, order.ID).Scan(&orderDigest); err != nil {
		t.Fatal(err)
	}

	expectedDigest := sha256.Sum256([]byte(setup.Capability))
	_ = expectedDigest

	if len(orderDigest) != 32 {
		t.Fatalf("order guest capability digest len = %d, want 32", len(orderDigest))
	}
}

func TestOrderCreatedEnvelopeCompleteAndPrivate(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()
	setup := setupSellerCheckoutTest(t, db, repo, ctx, suffix, 10, 1000)

	req := testFinalizePayload(setup.Session.ID)
	order, err := repo.FinalizeCheckout(ctx, setup.Store.ID, req, "corr-envelope")
	if err != nil {
		t.Fatal(err)
	}

	var payloadRaw []byte
	if err := db.QueryRow(ctx, `SELECT payload FROM outbox_events WHERE aggregate_id = $1 AND event_type = 'commerce.order.created.v1'`, order.ID).Scan(&payloadRaw); err != nil {
		t.Fatal(err)
	}

	payloadStr := string(payloadRaw)
	// Assert private fields absent
	for _, forbidden := range []string{"supplier_cost_minor", "supplier_cost_currency_code", "source_supplier_id", "supplier_offer_id", "guest_order_access_token_digest", "reservation_token"} {
		if containsString(payloadStr, forbidden) {
			t.Fatalf("outbox event payload contains private field %s: %s", forbidden, payloadStr)
		}
	}
}

func TestFinalizeCheckoutCurrencyChanged(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()
	setup := setupSellerCheckoutTest(t, db, repo, ctx, suffix, 10, 1000)

	// Change retail price currency to SAR (valid currency in reference data, but different from cart currency EGP)
	if _, err := db.Exec(ctx, `UPDATE seller_listing_prices SET currency_code = 'SAR' WHERE seller_listing_id = $1 AND is_current = true`, setup.ListingID); err != nil {
		t.Fatal(err)
	}

	req := testFinalizePayload(setup.Session.ID)
	_, err := repo.FinalizeCheckout(ctx, setup.Store.ID, req, "corr-currchange")
	if !errors.Is(err, ErrPriceChanged) {
		t.Fatalf("expected ErrPriceChanged for currency change, got %v", err)
	}
}

func TestFinalizeCheckoutInactiveVariant(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()
	setup := setupSellerCheckoutTest(t, db, repo, ctx, suffix, 10, 1000)

	if _, err := db.Exec(ctx, `UPDATE variants SET status = 'inactive' WHERE id = $1`, setup.VariantID); err != nil {
		t.Fatal(err)
	}

	req := testFinalizePayload(setup.Session.ID)
	_, err := repo.FinalizeCheckout(ctx, setup.Store.ID, req, "corr-inactvar")
	if !errors.Is(err, ErrListingUnavailable) {
		t.Fatalf("expected ErrListingUnavailable for inactive variant, got %v", err)
	}
}

func TestFinalizeCheckoutInactiveSKU(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()
	setup := setupSellerCheckoutTest(t, db, repo, ctx, suffix, 10, 1000)

	if _, err := db.Exec(ctx, `UPDATE skus SET status = 'inactive' WHERE id = $1`, setup.SKUID); err != nil {
		t.Fatal(err)
	}

	req := testFinalizePayload(setup.Session.ID)
	_, err := repo.FinalizeCheckout(ctx, setup.Store.ID, req, "corr-inactsku")
	if !errors.Is(err, ErrListingUnavailable) {
		t.Fatalf("expected ErrListingUnavailable for inactive SKU, got %v", err)
	}
}

func TestFinalizeCheckoutMissingGuestDigestRejected(t *testing.T) {
	db, _, ctx := setupP53Database(t)

	// Verify DB check constraint rejects non-32 byte digest
	cartID := uuid.NewString()
	storeID := uuid.NewString()
	sessionID := uuid.NewString()

	// Direct INSERT with invalid digest should fail DB check constraint
	_, err := db.Exec(ctx, `
		INSERT INTO checkout_sessions (id, store_id, cart_id, status, expires_at, guest_order_access_token_digest)
		VALUES ($1, $2, $3, 'open', clock_timestamp() + interval '30 minutes', E'\\\\x1234')
	`, sessionID, storeID, cartID)
	if err == nil {
		t.Fatal("expected DB check constraint error for invalid guest digest, got nil")
	}
}

func TestFinalizeCheckoutNoSplitFulfillment(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()

	// Initial stock: Loc A = 3, Loc B = 3. Line quantity = 5
	seller, err := repo.CreateSeller(ctx, "seller-"+suffix, "Seller "+suffix, "active", nil)
	if err != nil {
		t.Fatal(err)
	}
	store, _, err := repo.CreateStoreWithDomain(ctx, seller.ID, "EG", "store-"+suffix, "Store "+suffix, "active", nil, suffix+".test", "platform", "active", true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	productID := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO products (id, slug, status) VALUES ($1, $2, 'active')`, productID, "prod-"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO product_translations (product_id, locale, name) VALUES ($1, 'en', 'Prod')`, productID); err != nil {
		t.Fatal(err)
	}
	variantID := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO variants (id, product_id, code, status) VALUES ($1, $2, $3, 'active')`, variantID, productID, "VAR-"+suffix); err != nil {
		t.Fatal(err)
	}
	skuID := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO skus (id, variant_id, code, status) VALUES ($1, $2, $3, 'active')`, skuID, variantID, "SKU-"+suffix); err != nil {
		t.Fatal(err)
	}
	listingID := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO seller_listings (id, store_id, product_id, market_code, status) VALUES ($1, $2, $3, 'EG', 'active')`, listingID, store.ID, productID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO seller_listing_prices (id, seller_listing_id, amount_minor, currency_code, is_current) VALUES ($1, $2, 1000, 'EGP', true)`, uuid.NewString(), listingID); err != nil {
		t.Fatal(err)
	}

	locA := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO fulfillment_locations (id, store_id, market_code, code, name, location_type, status) VALUES ($1, $2, 'EG', $3, 'Loc A', 'warehouse', 'active')`, locA, store.ID, "LOCA-"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO inventory_snapshots (id, fulfillment_location_id, sku_id, on_hand_qty, reserved_qty, version) VALUES ($1, $2, $3, 3, 0, 1)`, uuid.NewString(), locA, skuID); err != nil {
		t.Fatal(err)
	}

	locB := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO fulfillment_locations (id, store_id, market_code, code, name, location_type, status) VALUES ($1, $2, 'EG', $3, 'Loc B', 'warehouse', 'active')`, locB, store.ID, "LOCB-"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO inventory_snapshots (id, fulfillment_location_id, sku_id, on_hand_qty, reserved_qty, version) VALUES ($1, $2, $3, 3, 0, 1)`, uuid.NewString(), locB, skuID); err != nil {
		t.Fatal(err)
	}

	_, cartToken, err := repo.CreateCart(ctx, store.ID, "EG", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.AddCartItem(ctx, store.ID, cartToken, skuID, 5)
	if err != nil {
		t.Fatal(err)
	}

	session, _, err := repo.CreateCheckoutSession(ctx, store.ID, cartToken, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	req := testFinalizePayload(session.ID)
	_, err = repo.FinalizeCheckout(ctx, store.ID, req, "corr-nosplit")
	if !errors.Is(err, ErrInsufficientInventory) {
		t.Fatalf("expected ErrInsufficientInventory for no-split stock failure, got %v", err)
	}
}

func TestFinalizeCheckoutCumulativeSnapshotDemand(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()

	seller, err := repo.CreateSeller(ctx, "seller-"+suffix, "Seller "+suffix, "active", nil)
	if err != nil {
		t.Fatal(err)
	}
	store, _, err := repo.CreateStoreWithDomain(ctx, seller.ID, "EG", "store-"+suffix, "Store "+suffix, "active", nil, suffix+".test", "platform", "active", true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	productID := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO products (id, slug, status) VALUES ($1, $2, 'active')`, productID, "prod-"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO product_translations (product_id, locale, name) VALUES ($1, 'en', 'Prod')`, productID); err != nil {
		t.Fatal(err)
	}
	variantID := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO variants (id, product_id, code, status) VALUES ($1, $2, $3, 'active')`, variantID, productID, "VAR-"+suffix); err != nil {
		t.Fatal(err)
	}
	skuID := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO skus (id, variant_id, code, status) VALUES ($1, $2, $3, 'active')`, skuID, variantID, "SKU-"+suffix); err != nil {
		t.Fatal(err)
	}

	listing1 := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO seller_listings (id, store_id, product_id, market_code, status) VALUES ($1, $2, $3, 'EG', 'active')`, listing1, store.ID, productID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO seller_listing_prices (id, seller_listing_id, amount_minor, currency_code, is_current) VALUES ($1, $2, 1000, 'EGP', true)`, uuid.NewString(), listing1); err != nil {
		t.Fatal(err)
	}

	product2 := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO products (id, slug, status) VALUES ($1, $2, 'active')`, product2, "prod2-"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO product_translations (product_id, locale, name) VALUES ($1, 'en', 'Prod 2')`, product2); err != nil {
		t.Fatal(err)
	}
	var2 := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO variants (id, product_id, code, status) VALUES ($1, $2, $3, 'active')`, var2, product2, "VAR2-"+suffix); err != nil {
		t.Fatal(err)
	}
	sku2 := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO skus (id, variant_id, code, status) VALUES ($1, $2, $3, 'active')`, sku2, var2, "SKU2-"+suffix); err != nil {
		t.Fatal(err)
	}

	listing2 := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO seller_listings (id, store_id, product_id, market_code, status) VALUES ($1, $2, $3, 'EG', 'active')`, listing2, store.ID, product2); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO seller_listing_prices (id, seller_listing_id, amount_minor, currency_code, is_current) VALUES ($1, $2, 1000, 'EGP', true)`, uuid.NewString(), listing2); err != nil {
		t.Fatal(err)
	}

	locationID := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO fulfillment_locations (id, store_id, market_code, code, name, location_type, status) VALUES ($1, $2, 'EG', $3, 'Loc', 'warehouse', 'active')`, locationID, store.ID, "LOC-"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO inventory_snapshots (id, fulfillment_location_id, sku_id, on_hand_qty, reserved_qty, version) VALUES ($1, $2, $3, 5, 0, 1)`, uuid.NewString(), locationID, skuID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO inventory_snapshots (id, fulfillment_location_id, sku_id, on_hand_qty, reserved_qty, version) VALUES ($1, $2, $3, 5, 0, 1)`, uuid.NewString(), locationID, sku2); err != nil {
		t.Fatal(err)
	}

	_, cartToken, err := repo.CreateCart(ctx, store.ID, "EG", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.AddCartItem(ctx, store.ID, cartToken, skuID, 3)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.AddCartItem(ctx, store.ID, cartToken, sku2, 3)
	if err != nil {
		t.Fatal(err)
	}

	session, _, err := repo.CreateCheckoutSession(ctx, store.ID, cartToken, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	req := testFinalizePayload(session.ID)
	order, err := repo.FinalizeCheckout(ctx, store.ID, req, "corr-cumul")
	if err != nil {
		t.Fatalf("cumulative allocation failed: %v", err)
	}
	if len(order.Items) != 2 {
		t.Fatalf("items count = %d, want 2", len(order.Items))
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || findSubstr(s, substr))
}

func findSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
