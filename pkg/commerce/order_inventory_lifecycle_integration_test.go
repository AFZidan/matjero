package commerce

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/matjeroapps/core/packages/database"
)

func createTestOrderWithInventory(t *testing.T, db *database.Pool, repo Repository, ctx context.Context, suffix string, initialOnHand, initialReserved, itemQty int64, deadlineDuration time.Duration) (Store, Order, string, string) {
	t.Helper()
	seller, err := repo.CreateSeller(ctx, "seller-"+suffix, "Seller "+suffix, "active", nil)
	if err != nil {
		t.Fatal(err)
	}
	store, _, err := repo.CreateStoreWithDomain(ctx, seller.ID, "EG", "store-"+suffix, "Store "+suffix, "active", nil, suffix+".test", "platform", "active", true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, cartToken, err := repo.CreateCart(ctx, store.ID, "EG", nil)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := repo.CreateCheckoutSession(ctx, store.ID, cartToken, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	uniq := uuid.NewString()[:8]
	locID := uuid.NewString()
	if _, err := db.Exec(ctx, `
		INSERT INTO fulfillment_locations (id, store_id, market_code, code, name, location_type, status)
		VALUES ($1, $2, 'EG', $3, 'Test Location', 'warehouse', 'active')
	`, locID, store.ID, "LOC-"+uniq); err != nil {
		t.Fatal(err)
	}

	productID := uuid.NewString()
	variantID := uuid.NewString()
	skuID := uuid.NewString()

	if _, err := db.Exec(ctx, `INSERT INTO products (id, slug, status) VALUES ($1, $2, 'active')`, productID, "slug-"+uniq); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO variants (id, product_id, code, status) VALUES ($1, $2, $3, 'active')`, variantID, productID, "VAR-"+uniq); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO skus (id, variant_id, code, status) VALUES ($1, $2, $3, 'active')`, skuID, variantID, "SKU-"+uniq); err != nil {
		t.Fatal(err)
	}

	snapID := uuid.NewString()
	if _, err := db.Exec(ctx, `
		INSERT INTO inventory_snapshots (id, sku_id, fulfillment_location_id, on_hand_qty, reserved_qty, version)
		VALUES ($1, $2, $3, $4, $5, 1)
	`, snapID, skuID, locID, initialOnHand, initialReserved); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().UTC().Add(deadlineDuration)

	resID := uuid.NewString()
	resToken := uuid.NewString()
	if _, err := db.Exec(ctx, `
		INSERT INTO inventory_reservations (id, reservation_token, inventory_snapshot_id, status, quantity, expires_at)
		VALUES ($1, $2, $3, 'held', $4, $5)
	`, resID, resToken, snapID, itemQty, deadline); err != nil {
		t.Fatal(err)
	}

	orderID := uuid.NewString()
	orderNumber := "#" + suffix[:8]
	order := Order{
		ID:                          orderID,
		OrderNumber:                 orderNumber,
		StoreID:                     store.ID,
		MarketCode:                  "EG",
		CheckoutSessionID:           session.ID,
		Status:                      OrderStatusPending,
		CurrencyCode:                "EGP",
		GuestOrderAccessTokenDigest: session.GuestOrderAccessTokenDigest,
		SubtotalMinor:               1000,
		TotalMinor:                  1000,
		ConfirmationDeadlineAt:      deadline,
		AggregateVersion:            1,
		CreatedAt:                   time.Now().UTC(),
		UpdatedAt:                   time.Now().UTC(),
		Items: []OrderItem{
			{
				ID:                     uuid.NewString(),
				OrderID:                orderID,
				FulfillmentLocationID:  locID,
				InventoryReservationID: resID,
				ProductTitleSnapshot:   "Test Product",
				SKUCodeSnapshot:        "SKU-TEST",
				UnitPriceMinor:         1000,
				CurrencyCode:           "EGP",
				Quantity:               itemQty,
				LineTotalMinor:         1000,
				CreatedAt:              time.Now().UTC(),
			},
		},
		Address: &OrderAddress{
			ID:            uuid.NewString(),
			OrderID:       orderID,
			AddressType:   AddressTypeShipping,
			RecipientName: "John Doe",
			AddressLine1:  "123 Street",
			City:          "Cairo",
			CountryCode:   "EG",
			CreatedAt:     time.Now().UTC(),
		},
	}

	createdOrder, err := repo.CreateOrder(ctx, nil, order)
	if err != nil {
		t.Fatal(err)
	}

	return store, createdOrder, snapID, resID
}

func TestSellerConfirmConsumption(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()
	store, order, snapID, resID := createTestOrderWithInventory(t, db, repo, ctx, suffix, 10, 2, 2, 30*time.Minute)

	actor := "seller-actor"
	confirmed, err := repo.ConfirmOrder(ctx, nil, store.ID, order.ID, &actor, "corr-1")
	if err != nil {
		t.Fatalf("ConfirmOrder returned error: %v", err)
	}
	if confirmed.Status != OrderStatusConfirmed {
		t.Errorf("expected status confirmed, got %s", confirmed.Status)
	}
	if confirmed.AggregateVersion != 2 {
		t.Errorf("expected aggregate_version 2, got %d", confirmed.AggregateVersion)
	}

	// Verify inventory snapshot: on_hand 10-2=8, reserved 2-2=0
	var onHand, reserved int64
	if err := db.QueryRow(ctx, `SELECT on_hand_qty, reserved_qty FROM inventory_snapshots WHERE id = $1`, snapID).Scan(&onHand, &reserved); err != nil {
		t.Fatal(err)
	}
	if onHand != 8 || reserved != 0 {
		t.Errorf("expected on_hand 8, reserved 0; got on_hand %d, reserved %d", onHand, reserved)
	}

	// Verify reservation state: consumed
	var resStatus string
	if err := db.QueryRow(ctx, `SELECT status FROM inventory_reservations WHERE id = $1`, resID).Scan(&resStatus); err != nil {
		t.Fatal(err)
	}
	if resStatus != ReservationStatusConsumed {
		t.Errorf("expected reservation status consumed, got %s", resStatus)
	}

	// Verify inventory movement
	var movType string
	var qtyDelta int64
	if err := db.QueryRow(ctx, `SELECT movement_type, quantity_delta FROM inventory_movements WHERE inventory_snapshot_id = $1 AND movement_type = 'reservation_consumed'`, snapID).Scan(&movType, &qtyDelta); err != nil {
		t.Fatal(err)
	}
	if qtyDelta != -2 {
		t.Errorf("expected quantity_delta -2, got %d", qtyDelta)
	}
}

func TestSellerCancelRestock(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()
	store, order, snapID, resID := createTestOrderWithInventory(t, db, repo, ctx, suffix, 10, 2, 2, 30*time.Minute)

	actor := "seller-actor"
	_, err := repo.ConfirmOrder(ctx, nil, store.ID, order.ID, &actor, "corr-1")
	if err != nil {
		t.Fatalf("ConfirmOrder returned error: %v", err)
	}

	reason := "out of stock item"
	cancelled, err := repo.CancelConfirmedOrder(ctx, nil, store.ID, order.ID, AuthoritySeller, &actor, &reason, "corr-2")
	if err != nil {
		t.Fatalf("CancelConfirmedOrder returned error: %v", err)
	}
	if cancelled.Status != OrderStatusCancelled {
		t.Errorf("expected status cancelled, got %s", cancelled.Status)
	}

	// Verify snapshot on_hand restocked back from 8 to 10, reserved stays 0
	var onHand, reserved int64
	if err := db.QueryRow(ctx, `SELECT on_hand_qty, reserved_qty FROM inventory_snapshots WHERE id = $1`, snapID).Scan(&onHand, &reserved); err != nil {
		t.Fatal(err)
	}
	if onHand != 10 || reserved != 0 {
		t.Errorf("expected on_hand 10, reserved 0; got on_hand %d, reserved %d", onHand, reserved)
	}

	// Reservation state remains consumed
	var resStatus string
	if err := db.QueryRow(ctx, `SELECT status FROM inventory_reservations WHERE id = $1`, resID).Scan(&resStatus); err != nil {
		t.Fatal(err)
	}
	if resStatus != ReservationStatusConsumed {
		t.Errorf("expected reservation status consumed, got %s", resStatus)
	}

	// Verify restock movement
	var qtyDelta int64
	if err := db.QueryRow(ctx, `SELECT quantity_delta FROM inventory_movements WHERE inventory_snapshot_id = $1 AND movement_type = 'order_cancellation_restock'`, snapID).Scan(&qtyDelta); err != nil {
		t.Fatal(err)
	}
	if qtyDelta != 2 {
		t.Errorf("expected quantity_delta +2 for restock, got %d", qtyDelta)
	}
}

func TestConfirmVsExpiryRace(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()
	store, order, snapID, _ := createTestOrderWithInventory(t, db, repo, ctx, suffix, 10, 2, 2, 30*time.Minute)

	startGate := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	actor := "seller-actor"
	var errConfirm, errExpiry error

	go func() {
		defer wg.Done()
		<-startGate
		_, errConfirm = repo.ConfirmOrder(ctx, nil, store.ID, order.ID, &actor, "corr-race")
	}()

	go func() {
		defer wg.Done()
		<-startGate
		_, errExpiry = repo.ExpirePendingOrder(ctx, nil, order.ID)
	}()

	close(startGate)
	wg.Wait()

	_ = errConfirm
	_ = errExpiry

	// Exactly one wins, the other fails or is a safe no-op
	updatedOrder, err := repo.GetOrderByID(ctx, nil, store.ID, order.ID)
	if err != nil {
		t.Fatal(err)
	}

	if updatedOrder.Status != OrderStatusConfirmed && updatedOrder.Status != OrderStatusCancelled {
		t.Errorf("expected order status confirmed or cancelled, got %s", updatedOrder.Status)
	}

	// Check reserved_qty in snapshot is 0 (decremented exactly once)
	var onHand, reserved int64
	if err := db.QueryRow(ctx, `SELECT on_hand_qty, reserved_qty FROM inventory_snapshots WHERE id = $1`, snapID).Scan(&onHand, &reserved); err != nil {
		t.Fatal(err)
	}
	if reserved != 0 {
		t.Errorf("expected reserved 0 (decremented once), got %d", reserved)
	}
}

func TestConfirmDeadlineBoundaries(t *testing.T) {
	db, repo, ctx := setupP53Database(t)

	// Case 71: Confirm before deadline -> valid
	suffixValid := uuid.NewString()
	storeV, orderV, _, _ := createTestOrderWithInventory(t, db, repo, ctx, suffixValid, 10, 2, 2, 10*time.Minute)
	actor := "seller"
	_, err := repo.ConfirmOrder(ctx, nil, storeV.ID, orderV.ID, &actor, "")
	if err != nil {
		t.Fatalf("expected valid confirm before deadline, got %v", err)
	}

	// Case 73, 74, 75: Confirm after deadline before scheduler -> rejected without inventory changes
	suffixLate := uuid.NewString()
	storeL, orderL, snapIDL, resIDL := createTestOrderWithInventory(t, db, repo, ctx, suffixLate, 10, 2, 2, -10*time.Minute)

	_, errLate := repo.ConfirmOrder(ctx, nil, storeL.ID, orderL.ID, &actor, "")
	if errLate != ErrInvalidTransition {
		t.Errorf("expected ErrInvalidTransition on late confirm, got %v", errLate)
	}

	// Verify reservation still held (Case 74)
	var resStatus string
	if err := db.QueryRow(ctx, `SELECT status FROM inventory_reservations WHERE id = $1`, resIDL).Scan(&resStatus); err != nil {
		t.Fatal(err)
	}
	if resStatus != ReservationStatusHeld {
		t.Errorf("expected reservation still held after rejected confirm, got %s", resStatus)
	}

	// Verify snapshot on_hand unchanged (Case 75)
	var onHand, reserved int64
	if err := db.QueryRow(ctx, `SELECT on_hand_qty, reserved_qty FROM inventory_snapshots WHERE id = $1`, snapIDL).Scan(&onHand, &reserved); err != nil {
		t.Fatal(err)
	}
	if onHand != 10 || reserved != 2 {
		t.Errorf("expected on_hand 10, reserved 2; got on_hand %d, reserved %d", onHand, reserved)
	}
}

func TestExpiryRetryIdempotency(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()
	_, order, snapID, resID := createTestOrderWithInventory(t, db, repo, ctx, suffix, 10, 2, 2, -10*time.Minute)

	// First expiry run
	expired1, err1 := repo.ExpirePendingOrder(ctx, nil, order.ID)
	if err1 != nil {
		t.Fatalf("first ExpirePendingOrder failed: %v", err1)
	}
	if expired1.Status != OrderStatusCancelled {
		t.Errorf("expected status cancelled, got %s", expired1.Status)
	}

	// Second expiry run (retry)
	_, err2 := repo.ExpirePendingOrder(ctx, nil, order.ID)
	if err2 != nil {
		t.Fatalf("second ExpirePendingOrder failed: %v", err2)
	}

	// Check reserved_qty is 0 (decremented only once from 2 to 0)
	var reserved int64
	if err := db.QueryRow(ctx, `SELECT reserved_qty FROM inventory_snapshots WHERE id = $1`, snapID).Scan(&reserved); err != nil {
		t.Fatal(err)
	}
	if reserved != 0 {
		t.Errorf("expected reserved_qty 0 after retry, got %d", reserved)
	}

	// Check reservation status is expired
	var resStatus string
	if err := db.QueryRow(ctx, `SELECT status FROM inventory_reservations WHERE id = $1`, resID).Scan(&resStatus); err != nil {
		t.Fatal(err)
	}
	if resStatus != ReservationStatusExpired {
		t.Errorf("expected reservation status expired, got %s", resStatus)
	}
}

func TestAllPhase5EnabledAndRejectedTransitions(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()
	store, order, _, _ := createTestOrderWithInventory(t, db, repo, ctx, suffix, 10, 2, 2, 30*time.Minute)
	actor := "seller"

	// Case 60: Rejected transition pending -> processing
	_, err := repo.AdvanceOrderStatus(ctx, nil, store.ID, order.ID, OrderStatusProcessing, AuthoritySeller, &actor, nil, "")
	if err != ErrInvalidTransition {
		t.Errorf("expected ErrInvalidTransition for pending -> processing, got %v", err)
	}

	// Case 59: Enabled pending -> confirmed
	confirmed, err := repo.ConfirmOrder(ctx, nil, store.ID, order.ID, &actor, "")
	if err != nil {
		t.Fatalf("ConfirmOrder failed: %v", err)
	}
	if confirmed.Status != OrderStatusConfirmed {
		t.Errorf("expected confirmed, got %s", confirmed.Status)
	}

	// Case 59: Enabled confirmed -> processing
	processing, err := repo.AdvanceOrderStatus(ctx, nil, store.ID, order.ID, OrderStatusProcessing, AuthoritySeller, &actor, nil, "")
	if err != nil {
		t.Fatalf("AdvanceOrderStatus to processing failed: %v", err)
	}
	if processing.Status != OrderStatusProcessing {
		t.Errorf("expected processing, got %s", processing.Status)
	}

	// Case 59: Enabled processing -> ready_for_shipping
	ready, err := repo.AdvanceOrderStatus(ctx, nil, store.ID, order.ID, OrderStatusReadyForShipping, AuthoritySeller, &actor, nil, "")
	if err != nil {
		t.Fatalf("AdvanceOrderStatus to ready_for_shipping failed: %v", err)
	}
	if ready.Status != OrderStatusReadyForShipping {
		t.Errorf("expected ready_for_shipping, got %s", ready.Status)
	}
}

func TestMultiItemAggregation(t *testing.T) {
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
	_, cartToken, err := repo.CreateCart(ctx, store.ID, "EG", nil)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := repo.CreateCheckoutSession(ctx, store.ID, cartToken, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	locID := uuid.NewString()
	if _, err := db.Exec(ctx, `
		INSERT INTO fulfillment_locations (id, store_id, market_code, code, name, location_type, status)
		VALUES ($1, $2, 'EG', $3, 'Test Location', 'warehouse', 'active')
	`, locID, store.ID, "LOC-"+suffix[:8]); err != nil {
		t.Fatal(err)
	}

	productID := uuid.NewString()
	variantID := uuid.NewString()
	skuID1 := uuid.NewString()

	if _, err := db.Exec(ctx, `INSERT INTO products (id, slug, status) VALUES ($1, $2, 'active')`, productID, "slug-"+suffix[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO variants (id, product_id, code, status) VALUES ($1, $2, $3, 'active')`, variantID, productID, "VAR-"+suffix[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO skus (id, variant_id, code, status) VALUES ($1, $2, $3, 'active')`, skuID1, variantID, "SKU-"+suffix[:8]); err != nil {
		t.Fatal(err)
	}

	snapID := uuid.NewString() // Single snapshot referenced by 2 items!
	if _, err := db.Exec(ctx, `
		INSERT INTO inventory_snapshots (id, sku_id, fulfillment_location_id, on_hand_qty, reserved_qty, version)
		VALUES ($1, $2, $3, 20, 5, 1)
	`, snapID, skuID1, locID); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().UTC().Add(30 * time.Minute)

	resID1 := uuid.NewString()
	resID2 := uuid.NewString()
	if _, err := db.Exec(ctx, `
		INSERT INTO inventory_reservations (id, reservation_token, inventory_snapshot_id, status, quantity, expires_at)
		VALUES ($1, $2, $3, 'held', 2, $4), ($5, $6, $3, 'held', 3, $4)
	`, resID1, uuid.NewString(), snapID, deadline, resID2, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	orderID := uuid.NewString()
	order := Order{
		ID:                          orderID,
		OrderNumber:                 "#" + suffix[:8],
		StoreID:                     store.ID,
		MarketCode:                  "EG",
		CheckoutSessionID:           session.ID,
		Status:                      OrderStatusPending,
		CurrencyCode:                "EGP",
		GuestOrderAccessTokenDigest: session.GuestOrderAccessTokenDigest,
		SubtotalMinor:               2000,
		TotalMinor:                  2000,
		ConfirmationDeadlineAt:      deadline,
		AggregateVersion:            1,
		CreatedAt:                   time.Now().UTC(),
		UpdatedAt:                   time.Now().UTC(),
		Items: []OrderItem{
			{
				ID:                     uuid.NewString(),
				OrderID:                orderID,
				FulfillmentLocationID:  locID,
				InventoryReservationID: resID1,
				ProductTitleSnapshot:   "Item 1",
				SKUCodeSnapshot:        "SKU-1",
				UnitPriceMinor:         1000,
				CurrencyCode:           "EGP",
				Quantity:               2,
				LineTotalMinor:         1000,
				CreatedAt:              time.Now().UTC(),
			},
			{
				ID:                     uuid.NewString(),
				OrderID:                orderID,
				FulfillmentLocationID:  locID,
				InventoryReservationID: resID2,
				ProductTitleSnapshot:   "Item 2",
				SKUCodeSnapshot:        "SKU-1",
				UnitPriceMinor:         1000,
				CurrencyCode:           "EGP",
				Quantity:               3,
				LineTotalMinor:         1000,
				CreatedAt:              time.Now().UTC(),
			},
		},
	}

	_, err = repo.CreateOrder(ctx, nil, order)
	if err != nil {
		t.Fatal(err)
	}

	actor := "seller"
	_, err = repo.ConfirmOrder(ctx, nil, store.ID, order.ID, &actor, "")
	if err != nil {
		t.Fatalf("ConfirmOrder failed: %v", err)
	}

	// Verify snapshot on_hand 20-5=15, reserved 5-5=0 (Matrix Case 116)
	var onHand, reserved int64
	if err := db.QueryRow(ctx, `SELECT on_hand_qty, reserved_qty FROM inventory_snapshots WHERE id = $1`, snapID).Scan(&onHand, &reserved); err != nil {
		t.Fatal(err)
	}
	if onHand != 15 || reserved != 0 {
		t.Errorf("expected aggregated on_hand 15, reserved 0; got on_hand %d, reserved %d", onHand, reserved)
	}

	// Verify exactly ONE movement created for snapID with quantity_delta -5
	var movCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM inventory_movements WHERE inventory_snapshot_id = $1 AND movement_type = 'reservation_consumed'`, snapID).Scan(&movCount); err != nil {
		t.Fatal(err)
	}
	if movCount != 1 {
		t.Errorf("expected exactly 1 aggregated movement, got %d", movCount)
	}
}

func TestCriticalRegressionPostLockDecisionNow(t *testing.T) {
	db, repo, ctx := setupP53Database(t)
	suffix := uuid.NewString()

	// Deadline expires in 1 second
	store, order, _, _ := createTestOrderWithInventory(t, db, repo, ctx, suffix, 10, 2, 2, 100*time.Millisecond)

	// Hold Order lock in Tx A
	txA, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer txA.Rollback(ctx)

	var lockedID string
	if err := txA.QueryRow(ctx, `SELECT id FROM orders WHERE id = $1 FOR UPDATE`, order.ID).Scan(&lockedID); err != nil {
		t.Fatal(err)
	}

	actor := "seller"
	var errConfirm error
	done := make(chan struct{})

	go func() {
		// ConfirmOrder starts in Tx B
		_, errConfirm = repo.ConfirmOrder(ctx, nil, store.ID, order.ID, &actor, "")
		close(done)
	}()

	// Wait for deadline to pass while Tx A holds the lock
	time.Sleep(300 * time.Millisecond)

	// Release Tx A lock
	if err := txA.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	<-done

	// ConfirmOrder acquired lock after deadline passed, so decision_now = clock_timestamp() > deadline -> must reject!
	if errConfirm != ErrInvalidTransition {
		t.Errorf("expected ErrInvalidTransition on lock-linearized late confirm, got %v", errConfirm)
	}
}
