package commerce

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DBExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (r Repository) getExec(exec DBExecutor) DBExecutor {
	if exec != nil {
		return exec
	}
	return r.pool
}

// AllocateOrderNumber atomically allocates the next sequential order number for a Store
// using store_order_sequences. The returned order number is formatted as #100001.
func (r Repository) AllocateOrderNumber(ctx context.Context, exec DBExecutor, storeID string) (string, error) {
	if strings.TrimSpace(storeID) == "" {
		return "", ErrInvalidInput
	}
	db := r.getExec(exec)
	var nextVal int64
	err := db.QueryRow(ctx, `
		INSERT INTO store_order_sequences (store_id, next_value)
		VALUES ($1, 100002)
		ON CONFLICT (store_id) DO UPDATE
		SET next_value = store_order_sequences.next_value + 1
		RETURNING next_value - 1
	`, storeID).Scan(&nextVal)
	if err != nil {
		return "", translatePGError(err, "allocate order number")
	}
	return fmt.Sprintf("#%d", nextVal), nil
}

// CreateOrder persists an Order, its items, and its address within the provided DBExecutor context.
func (r Repository) CreateOrder(ctx context.Context, exec DBExecutor, order Order) (Order, error) {
	if exec == nil {
		var created Order
		err := r.withTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			created, err = r.createOrderExec(ctx, tx, order)
			return err
		})
		return created, err
	}
	return r.createOrderExec(ctx, exec, order)
}

func (r Repository) createOrderExec(ctx context.Context, db DBExecutor, order Order) (Order, error) {
	if strings.TrimSpace(order.ID) == "" ||
		strings.TrimSpace(order.OrderNumber) == "" ||
		strings.TrimSpace(order.StoreID) == "" ||
		strings.TrimSpace(order.MarketCode) == "" ||
		strings.TrimSpace(order.CheckoutSessionID) == "" ||
		strings.TrimSpace(order.Status) == "" ||
		strings.TrimSpace(order.CurrencyCode) == "" ||
		order.SubtotalMinor < 0 ||
		order.TotalMinor < 0 ||
		order.ConfirmationDeadlineAt.IsZero() {
		return Order{}, ErrInvalidInput
	}

	// Enforce guest/customer invariant
	if order.CustomerID == nil && len(order.GuestOrderAccessTokenDigest) == 0 {
		return Order{}, ErrInvalidInput
	}
	if order.CustomerID == nil && len(order.GuestOrderAccessTokenDigest) != sha256.Size {
		return Order{}, ErrInvalidInput
	}

	if order.AggregateVersion == 0 {
		order.AggregateVersion = 1
	}
	now := time.Now().UTC()
	if order.CreatedAt.IsZero() {
		order.CreatedAt = now
	}
	if order.UpdatedAt.IsZero() {
		order.UpdatedAt = now
	}

	_, err := db.Exec(ctx, `
		INSERT INTO orders (
			id, order_number, store_id, market_code, customer_id, checkout_session_id,
			status, currency_code, guest_order_access_token_digest, subtotal_minor,
			total_minor, confirmation_deadline_at, cancellation_reason, aggregate_version,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`, order.ID, order.OrderNumber, order.StoreID, order.MarketCode, order.CustomerID,
		order.CheckoutSessionID, order.Status, order.CurrencyCode, order.GuestOrderAccessTokenDigest,
		order.SubtotalMinor, order.TotalMinor, order.ConfirmationDeadlineAt,
		order.CancellationReason, order.AggregateVersion, order.CreatedAt, order.UpdatedAt)
	if err != nil {
		return Order{}, translatePGError(err, "insert order")
	}

	for i, item := range order.Items {
		if strings.TrimSpace(item.ID) == "" {
			item.ID = uuid.NewString()
		}
		item.OrderID = order.ID
		if item.Quantity <= 0 || item.UnitPriceMinor < 0 || item.LineTotalMinor < 0 || strings.TrimSpace(item.FulfillmentLocationID) == "" || strings.TrimSpace(item.InventoryReservationID) == "" || strings.TrimSpace(item.ProductTitleSnapshot) == "" || strings.TrimSpace(item.SKUCodeSnapshot) == "" {
			return Order{}, ErrInvalidInput
		}
		if item.CurrencyCode == "" {
			item.CurrencyCode = order.CurrencyCode
		}

		// Supplier Commercial Snapshot Invariant check
		hasSupplierOffer := item.SupplierOfferID != nil
		hasSourceSupplier := item.SourceSupplierID != nil
		hasSupplierCost := item.SupplierCostMinor != nil
		hasSupplierCostCurr := item.SupplierCostCurrencyCode != nil

		isSupplierBacked := hasSupplierOffer && hasSourceSupplier && hasSupplierCost && hasSupplierCostCurr
		isSellerOwned := !hasSupplierOffer && !hasSourceSupplier && !hasSupplierCost && !hasSupplierCostCurr

		if !isSupplierBacked && !isSellerOwned {
			return Order{}, ErrInvalidInput
		}

		if isSupplierBacked {
			if *item.SupplierCostMinor < 0 || *item.SupplierCostCurrencyCode != item.CurrencyCode {
				return Order{}, ErrInvalidInput
			}
		}

		if item.CreatedAt.IsZero() {
			item.CreatedAt = order.CreatedAt
		}

		_, err := db.Exec(ctx, `
			INSERT INTO order_items (
				id, order_id, seller_listing_id, product_id, variant_id, sku_id,
				supplier_offer_id, source_supplier_id, fulfillment_location_id,
				inventory_reservation_id, product_title_snapshot, sku_code_snapshot,
				unit_price_minor, currency_code, quantity, line_total_minor,
				supplier_cost_minor, supplier_cost_currency_code, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		`, item.ID, item.OrderID, item.SellerListingID, item.ProductID, item.VariantID, item.SKUID,
			item.SupplierOfferID, item.SourceSupplierID, item.FulfillmentLocationID,
			item.InventoryReservationID, item.ProductTitleSnapshot, item.SKUCodeSnapshot,
			item.UnitPriceMinor, item.CurrencyCode, item.Quantity, item.LineTotalMinor,
			item.SupplierCostMinor, item.SupplierCostCurrencyCode, item.CreatedAt)
		if err != nil {
			return Order{}, translatePGError(err, "insert order item")
		}
		order.Items[i] = item
	}

	if order.Address != nil {
		addr := order.Address
		if strings.TrimSpace(addr.ID) == "" {
			addr.ID = uuid.NewString()
		}
		addr.OrderID = order.ID
		if addr.AddressType == "" {
			addr.AddressType = AddressTypeShipping
		}
		if strings.TrimSpace(addr.RecipientName) == "" ||
			strings.TrimSpace(addr.AddressLine1) == "" ||
			strings.TrimSpace(addr.City) == "" ||
			len(strings.TrimSpace(addr.CountryCode)) != 2 {
			return Order{}, ErrInvalidInput
		}
		if addr.CreatedAt.IsZero() {
			addr.CreatedAt = order.CreatedAt
		}

		_, err := db.Exec(ctx, `
			INSERT INTO order_addresses (
				id, order_id, address_type, recipient_name, phone, address_line_1,
				address_line_2, city, region, postal_code, country_code, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`, addr.ID, addr.OrderID, addr.AddressType, addr.RecipientName, addr.Phone,
			addr.AddressLine1, addr.AddressLine2, addr.City, addr.Region,
			addr.PostalCode, addr.CountryCode, addr.CreatedAt)
		if err != nil {
			return Order{}, translatePGError(err, "insert order address")
		}
		order.Address = addr
	}

	return order, nil
}

// GetOrderByID loads an order by ID within a Store tenant boundary.
func (r Repository) GetOrderByID(ctx context.Context, exec DBExecutor, storeID, orderID string) (Order, error) {
	if strings.TrimSpace(storeID) == "" || strings.TrimSpace(orderID) == "" {
		return Order{}, ErrInvalidInput
	}
	db := r.getExec(exec)

	var order Order
	err := db.QueryRow(ctx, `
		SELECT id, order_number, store_id, market_code, customer_id, checkout_session_id,
		       status, currency_code, guest_order_access_token_digest, subtotal_minor,
		       total_minor, confirmation_deadline_at, cancellation_reason, aggregate_version,
		       created_at, updated_at
		FROM orders
		WHERE id = $1 AND store_id = $2
	`, orderID, storeID).Scan(
		&order.ID, &order.OrderNumber, &order.StoreID, &order.MarketCode, &order.CustomerID,
		&order.CheckoutSessionID, &order.Status, &order.CurrencyCode, &order.GuestOrderAccessTokenDigest,
		&order.SubtotalMinor, &order.TotalMinor, &order.ConfirmationDeadlineAt,
		&order.CancellationReason, &order.AggregateVersion, &order.CreatedAt, &order.UpdatedAt,
	)
	if err != nil {
		return Order{}, translatePGError(err, "get order by id")
	}

	items, err := r.loadOrderItems(ctx, db, order.ID)
	if err != nil {
		return Order{}, err
	}
	order.Items = items

	addr, err := r.loadOrderAddress(ctx, db, order.ID)
	if err != nil && err != ErrNotFound {
		return Order{}, err
	}
	if err == nil {
		order.Address = &addr
	}

	return order, nil
}

// GetOrderByNumber loads an order by order_number within a Store tenant boundary.
func (r Repository) GetOrderByNumber(ctx context.Context, exec DBExecutor, storeID, orderNumber string) (Order, error) {
	if strings.TrimSpace(storeID) == "" || strings.TrimSpace(orderNumber) == "" {
		return Order{}, ErrInvalidInput
	}
	db := r.getExec(exec)

	var order Order
	err := db.QueryRow(ctx, `
		SELECT id, order_number, store_id, market_code, customer_id, checkout_session_id,
		       status, currency_code, guest_order_access_token_digest, subtotal_minor,
		       total_minor, confirmation_deadline_at, cancellation_reason, aggregate_version,
		       created_at, updated_at
		FROM orders
		WHERE order_number = $1 AND store_id = $2
	`, orderNumber, storeID).Scan(
		&order.ID, &order.OrderNumber, &order.StoreID, &order.MarketCode, &order.CustomerID,
		&order.CheckoutSessionID, &order.Status, &order.CurrencyCode, &order.GuestOrderAccessTokenDigest,
		&order.SubtotalMinor, &order.TotalMinor, &order.ConfirmationDeadlineAt,
		&order.CancellationReason, &order.AggregateVersion, &order.CreatedAt, &order.UpdatedAt,
	)
	if err != nil {
		return Order{}, translatePGError(err, "get order by number")
	}

	items, err := r.loadOrderItems(ctx, db, order.ID)
	if err != nil {
		return Order{}, err
	}
	order.Items = items

	addr, err := r.loadOrderAddress(ctx, db, order.ID)
	if err != nil && err != ErrNotFound {
		return Order{}, err
	}
	if err == nil {
		order.Address = &addr
	}

	return order, nil
}

func (r Repository) loadOrderItems(ctx context.Context, db DBExecutor, orderID string) ([]OrderItem, error) {
	rows, err := db.Query(ctx, `
		SELECT id, order_id, seller_listing_id, product_id, variant_id, sku_id,
		       supplier_offer_id, source_supplier_id, fulfillment_location_id,
		       inventory_reservation_id, product_title_snapshot, sku_code_snapshot,
		       unit_price_minor, currency_code, quantity, line_total_minor,
		       supplier_cost_minor, supplier_cost_currency_code, created_at
		FROM order_items
		WHERE order_id = $1
		ORDER BY id ASC
	`, orderID)
	if err != nil {
		return nil, translatePGError(err, "load order items")
	}
	defer rows.Close()

	var items []OrderItem
	for rows.Next() {
		var item OrderItem
		err := rows.Scan(
			&item.ID, &item.OrderID, &item.SellerListingID, &item.ProductID, &item.VariantID, &item.SKUID,
			&item.SupplierOfferID, &item.SourceSupplierID, &item.FulfillmentLocationID,
			&item.InventoryReservationID, &item.ProductTitleSnapshot, &item.SKUCodeSnapshot,
			&item.UnitPriceMinor, &item.CurrencyCode, &item.Quantity, &item.LineTotalMinor,
			&item.SupplierCostMinor, &item.SupplierCostCurrencyCode, &item.CreatedAt,
		)
		if err != nil {
			return nil, translatePGError(err, "scan order item")
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, translatePGError(err, "iterate order items")
	}
	return items, nil
}

func (r Repository) loadOrderAddress(ctx context.Context, db DBExecutor, orderID string) (OrderAddress, error) {
	var addr OrderAddress
	err := db.QueryRow(ctx, `
		SELECT id, order_id, address_type, recipient_name, phone, address_line_1,
		       address_line_2, city, region, postal_code, country_code, created_at
		FROM order_addresses
		WHERE order_id = $1
	`, orderID).Scan(
		&addr.ID, &addr.OrderID, &addr.AddressType, &addr.RecipientName, &addr.Phone,
		&addr.AddressLine1, &addr.AddressLine2, &addr.City, &addr.Region,
		&addr.PostalCode, &addr.CountryCode, &addr.CreatedAt,
	)
	if err != nil {
		return OrderAddress{}, translatePGError(err, "load order address")
	}
	return addr, nil
}

// UpdateOrderStatus performs state transition validation and updates status, version, and appends a timeline entry.
func (r Repository) UpdateOrderStatus(
	ctx context.Context,
	exec DBExecutor,
	storeID, orderID string,
	targetStatus string,
	authority TransitionAuthority,
	actorSubject *string,
	reason *string,
	decisionNow time.Time,
) (Order, error) {
	if strings.TrimSpace(storeID) == "" || strings.TrimSpace(orderID) == "" || strings.TrimSpace(targetStatus) == "" {
		return Order{}, ErrInvalidInput
	}
	if decisionNow.IsZero() {
		return Order{}, ErrInvalidInput
	}

	if exec == nil {
		var updated Order
		err := r.withTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			updated, err = r.updateOrderStatusExec(ctx, tx, storeID, orderID, targetStatus, authority, actorSubject, reason, decisionNow)
			return err
		})
		return updated, err
	}
	return r.updateOrderStatusExec(ctx, exec, storeID, orderID, targetStatus, authority, actorSubject, reason, decisionNow)
}

func (r Repository) updateOrderStatusExec(
	ctx context.Context,
	db DBExecutor,
	storeID, orderID string,
	targetStatus string,
	authority TransitionAuthority,
	actorSubject *string,
	reason *string,
	decisionNow time.Time,
) (Order, error) {
	var order Order
	err := db.QueryRow(ctx, `
		SELECT id, order_number, store_id, market_code, customer_id, checkout_session_id,
		       status, currency_code, guest_order_access_token_digest, subtotal_minor,
		       total_minor, confirmation_deadline_at, cancellation_reason, aggregate_version,
		       created_at, updated_at
		FROM orders
		WHERE id = $1 AND store_id = $2
		FOR UPDATE
	`, orderID, storeID).Scan(
		&order.ID, &order.OrderNumber, &order.StoreID, &order.MarketCode, &order.CustomerID,
		&order.CheckoutSessionID, &order.Status, &order.CurrencyCode, &order.GuestOrderAccessTokenDigest,
		&order.SubtotalMinor, &order.TotalMinor, &order.ConfirmationDeadlineAt,
		&order.CancellationReason, &order.AggregateVersion, &order.CreatedAt, &order.UpdatedAt,
	)
	if err != nil {
		return Order{}, translatePGError(err, "lock order for transition")
	}

	if err := ValidateOrderTransition(order.Status, authority, targetStatus, order.ConfirmationDeadlineAt, decisionNow); err != nil {
		return Order{}, err
	}

	fromStatus := order.Status
	order.Status = targetStatus
	order.CancellationReason = reason
	order.AggregateVersion++
	order.UpdatedAt = decisionNow

	commandTag, err := db.Exec(ctx, `
		UPDATE orders
		SET status = $1, cancellation_reason = $2, aggregate_version = $3, updated_at = $4
		WHERE id = $5 AND store_id = $6 AND status = $7
	`, order.Status, order.CancellationReason, order.AggregateVersion, order.UpdatedAt, order.ID, order.StoreID, fromStatus)
	if err != nil {
		return Order{}, translatePGError(err, "update order status")
	}
	if commandTag.RowsAffected() == 0 {
		return Order{}, ErrConflict
	}

	// Append timeline
	timelineID := uuid.NewString()
	_, err = db.Exec(ctx, `
		INSERT INTO order_timeline (id, order_id, from_status, to_status, actor_type, actor_subject, reason, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, timelineID, order.ID, fromStatus, order.Status, string(authority), actorSubject, reason, decisionNow)
	if err != nil {
		return Order{}, translatePGError(err, "append order timeline")
	}

	items, err := r.loadOrderItems(ctx, db, order.ID)
	if err == nil {
		order.Items = items
	}
	addr, err := r.loadOrderAddress(ctx, db, order.ID)
	if err == nil {
		order.Address = &addr
	}

	return order, nil
}

// AppendTimeline appends an immutable entry to order_timeline.
func (r Repository) AppendTimeline(ctx context.Context, exec DBExecutor, timeline OrderTimeline) (OrderTimeline, error) {
	if strings.TrimSpace(timeline.OrderID) == "" || strings.TrimSpace(timeline.ToStatus) == "" || strings.TrimSpace(timeline.ActorType) == "" {
		return OrderTimeline{}, ErrInvalidInput
	}
	db := r.getExec(exec)
	if strings.TrimSpace(timeline.ID) == "" {
		timeline.ID = uuid.NewString()
	}
	if timeline.CreatedAt.IsZero() {
		timeline.CreatedAt = time.Now().UTC()
	}

	_, err := db.Exec(ctx, `
		INSERT INTO order_timeline (id, order_id, from_status, to_status, actor_type, actor_subject, reason, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, timeline.ID, timeline.OrderID, timeline.FromStatus, timeline.ToStatus, timeline.ActorType, timeline.ActorSubject, timeline.Reason, timeline.Metadata, timeline.CreatedAt)
	if err != nil {
		return OrderTimeline{}, translatePGError(err, "append timeline")
	}
	return timeline, nil
}

// CreateNote adds an internal note to an Order.
func (r Repository) CreateNote(ctx context.Context, exec DBExecutor, note OrderNote) (OrderNote, error) {
	if strings.TrimSpace(note.OrderID) == "" || strings.TrimSpace(note.AuthorSubject) == "" || strings.TrimSpace(note.Body) == "" {
		return OrderNote{}, ErrInvalidInput
	}
	if note.Visibility == "" {
		note.Visibility = NoteVisibilityInternal
	}
	if note.Visibility != NoteVisibilityInternal {
		return OrderNote{}, ErrInvalidInput
	}
	db := r.getExec(exec)
	if strings.TrimSpace(note.ID) == "" {
		note.ID = uuid.NewString()
	}
	if note.CreatedAt.IsZero() {
		note.CreatedAt = time.Now().UTC()
	}

	_, err := db.Exec(ctx, `
		INSERT INTO order_notes (id, order_id, author_subject, visibility, body, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, note.ID, note.OrderID, note.AuthorSubject, note.Visibility, note.Body, note.CreatedAt)
	if err != nil {
		return OrderNote{}, translatePGError(err, "create order note")
	}
	return note, nil
}

// GetNotes lists notes for an Order within a Store boundary.
func (r Repository) GetNotes(ctx context.Context, exec DBExecutor, storeID, orderID string) ([]OrderNote, error) {
	if strings.TrimSpace(storeID) == "" || strings.TrimSpace(orderID) == "" {
		return nil, ErrInvalidInput
	}
	db := r.getExec(exec)
	// Verify order store ownership
	var foundID string
	err := db.QueryRow(ctx, `SELECT id FROM orders WHERE id = $1 AND store_id = $2`, orderID, storeID).Scan(&foundID)
	if err != nil {
		return nil, translatePGError(err, "verify order store for notes")
	}

	rows, err := db.Query(ctx, `
		SELECT id, order_id, author_subject, visibility, body, created_at
		FROM order_notes
		WHERE order_id = $1
		ORDER BY created_at DESC
	`, orderID)
	if err != nil {
		return nil, translatePGError(err, "get order notes")
	}
	defer rows.Close()

	var notes []OrderNote
	for rows.Next() {
		var n OrderNote
		if err := rows.Scan(&n.ID, &n.OrderID, &n.AuthorSubject, &n.Visibility, &n.Body, &n.CreatedAt); err != nil {
			return nil, translatePGError(err, "scan order note")
		}
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, translatePGError(err, "iterate order notes")
	}
	return notes, nil
}
