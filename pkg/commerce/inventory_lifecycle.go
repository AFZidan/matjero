package commerce

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	ReservationStatusHeld     = "held"
	ReservationStatusConsumed = "consumed"
	ReservationStatusReleased = "released"
	ReservationStatusExpired  = "expired"
)

const (
	MovementTypeReservationHeld          = "reservation_held"
	MovementTypeReservationReleased      = "reservation_released"
	MovementTypeReservationExpired       = "reservation_expired"
	MovementTypeReservationConsumed      = "reservation_consumed"
	MovementTypeOrderCancellationRestock = "order_cancellation_restock"
)

// AllocateLineSnapshot selects the lowest eligible InventorySnapshot by id ASC
// that can fulfill the entire requested quantity, taking into account cumulative
// allocations already made to snapshots within the same transaction.
func AllocateLineSnapshot(candidates []InventorySnapshot, quantity int64, cumulativeAllocations map[string]int64) (InventorySnapshot, error) {
	if quantity <= 0 {
		return InventorySnapshot{}, ErrInvalidInput
	}

	// Sort candidates deterministically by ID ASC
	sorted := make([]InventorySnapshot, len(candidates))
	copy(sorted, candidates)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})

	for _, cand := range sorted {
		alreadyAllocated := cumulativeAllocations[cand.ID]
		available := cand.OnHandQty - cand.ReservedQty - alreadyAllocated
		if available >= quantity {
			return cand, nil
		}
	}
	return InventorySnapshot{}, ErrInsufficientInventory
}

// HoldReservationParams contains input for creating a held inventory reservation.
type HoldReservationParams struct {
	SnapshotID             string
	Quantity               int64
	ReservationToken       string
	ConfirmationDeadlineAt time.Time
	DecisionNow            time.Time
}

// HoldReservation creates a held inventory reservation, bumps snapshot reserved_qty,
// and inserts a reservation_held movement within the provided DBExecutor transaction.
func (r Repository) HoldReservation(ctx context.Context, exec DBExecutor, params HoldReservationParams) (InventoryReservation, error) {
	tx, err := requireTx(exec)
	if err != nil {
		return InventoryReservation{}, err
	}

	if strings.TrimSpace(params.SnapshotID) == "" ||
		params.Quantity <= 0 ||
		strings.TrimSpace(params.ReservationToken) == "" ||
		params.ConfirmationDeadlineAt.IsZero() ||
		params.DecisionNow.IsZero() ||
		!params.ConfirmationDeadlineAt.After(params.DecisionNow) {
		return InventoryReservation{}, ErrInvalidInput
	}

	// Lock snapshot FOR UPDATE
	var snap InventorySnapshot
	err = tx.QueryRow(ctx, `
		SELECT id, fulfillment_location_id, sku_id, on_hand_qty, reserved_qty, version, created_at, updated_at
		FROM inventory_snapshots
		WHERE id = $1
		FOR UPDATE
	`, params.SnapshotID).Scan(
		&snap.ID, &snap.FulfillmentLocationID, &snap.SKUID, &snap.OnHandQty,
		&snap.ReservedQty, &snap.Version, &snap.CreatedAt, &snap.UpdatedAt,
	)
	if err != nil {
		return InventoryReservation{}, translatePGError(err, "lock snapshot for hold reservation")
	}

	if snap.OnHandQty-snap.ReservedQty < params.Quantity {
		return InventoryReservation{}, ErrInsufficientInventory
	}

	newReservedQty := snap.ReservedQty + params.Quantity
	if newReservedQty > snap.OnHandQty {
		return InventoryReservation{}, ErrInsufficientInventory
	}
	newVersion := snap.Version + 1

	cmdTag, err := tx.Exec(ctx, `
		UPDATE inventory_snapshots
		SET reserved_qty = $1, version = $2, updated_at = $3
		WHERE id = $4 AND version = $5
	`, newReservedQty, newVersion, params.DecisionNow, snap.ID, snap.Version)
	if err != nil {
		return InventoryReservation{}, translatePGError(err, "update inventory snapshot for hold")
	}
	if cmdTag.RowsAffected() == 0 {
		return InventoryReservation{}, ErrConflict
	}

	resID := uuid.NewString()
	res := InventoryReservation{
		ID:                  resID,
		InventorySnapshotID: snap.ID,
		Quantity:            params.Quantity,
		Status:              ReservationStatusHeld,
		ReservationToken:    params.ReservationToken,
		ExpiresAt:           &params.ConfirmationDeadlineAt,
		CreatedAt:           params.DecisionNow,
		UpdatedAt:           params.DecisionNow,
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO inventory_reservations (id, inventory_snapshot_id, quantity, status, reservation_token, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, res.ID, res.InventorySnapshotID, res.Quantity, res.Status, res.ReservationToken, res.ExpiresAt, res.CreatedAt, res.UpdatedAt)
	if err != nil {
		return InventoryReservation{}, translatePGError(err, "insert inventory reservation")
	}

	movID := uuid.NewString()
	_, err = tx.Exec(ctx, `
		INSERT INTO inventory_movements (id, inventory_snapshot_id, movement_type, quantity_delta, on_hand_qty, reserved_qty, reason, principal_subject, correlation_id, causation_id, created_at)
		VALUES ($1, $2, $3, 0, $4, $5, 'checkout_hold', 'checkout', '', '', $6)
	`, movID, snap.ID, MovementTypeReservationHeld, snap.OnHandQty, newReservedQty, params.DecisionNow)
	if err != nil {
		return InventoryReservation{}, translatePGError(err, "insert reservation_held movement")
	}

	return res, nil
}

// SnapshotQuantityDelta stores aggregated item quantity changes per inventory snapshot.
type SnapshotQuantityDelta struct {
	SnapshotID        string
	AggregateQuantity int64
}

// AggregateReservationsBySnapshot groups reservations by snapshot ID and sums quantities.
func AggregateReservationsBySnapshot(reservations []InventoryReservation) []SnapshotQuantityDelta {
	aggMap := make(map[string]int64)
	for _, res := range reservations {
		aggMap[res.InventorySnapshotID] += res.Quantity
	}

	// Sort deterministically by SnapshotID ASC
	snapshotIDs := make([]string, 0, len(aggMap))
	for id := range aggMap {
		snapshotIDs = append(snapshotIDs, id)
	}
	sort.Strings(snapshotIDs)

	res := make([]SnapshotQuantityDelta, 0, len(snapshotIDs))
	for _, id := range snapshotIDs {
		res = append(res, SnapshotQuantityDelta{
			SnapshotID:        id,
			AggregateQuantity: aggMap[id],
		})
	}
	return res
}
