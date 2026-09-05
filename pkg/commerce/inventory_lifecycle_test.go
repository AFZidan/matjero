package commerce

import (
	"testing"
)

func TestAllocateLineSnapshot(t *testing.T) {
	candidates := []InventorySnapshot{
		{ID: "snap-c", FulfillmentLocationID: "loc-3", SKUID: "sku-1", OnHandQty: 10, ReservedQty: 0},
		{ID: "snap-a", FulfillmentLocationID: "loc-1", SKUID: "sku-1", OnHandQty: 5, ReservedQty: 0},
		{ID: "snap-b", FulfillmentLocationID: "loc-2", SKUID: "sku-1", OnHandQty: 10, ReservedQty: 0},
	}

	cumulative := make(map[string]int64)

	// Test Matrix Case 2: Select lowest inventory_snapshot.id ASC (snap-a)
	allocated, err := AllocateLineSnapshot(candidates, 3, cumulative)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allocated.ID != "snap-a" {
		t.Errorf("expected snap-a (lowest ID ASC), got %s", allocated.ID)
	}

	// Test Matrix Case 113: Cumulative demand within transaction
	cumulative["snap-a"] += 3 // 3 allocated to snap-a out of 5 capacity

	// Next line requests 3 units. snap-a has only 5-0-3 = 2 left, so it falls back to snap-b (Matrix Case 114)
	allocated2, err := AllocateLineSnapshot(candidates, 3, cumulative)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allocated2.ID != "snap-b" {
		t.Errorf("expected snap-b as fallback, got %s", allocated2.ID)
	}

	// Test Matrix Case 112: No split fulfillment (require 15 units when snap-a has 2, snap-b has 10, snap-c has 10)
	_, err = AllocateLineSnapshot(candidates, 15, cumulative)
	if err != ErrInsufficientInventory {
		t.Errorf("expected ErrInsufficientInventory for unsplit line, got %v", err)
	}
}

func TestAggregateReservationsBySnapshot(t *testing.T) {
	reservations := []InventoryReservation{
		{ID: "res-1", InventorySnapshotID: "snap-b", Quantity: 2},
		{ID: "res-2", InventorySnapshotID: "snap-a", Quantity: 3},
		{ID: "res-3", InventorySnapshotID: "snap-b", Quantity: 4},
	}

	agg := AggregateReservationsBySnapshot(reservations)
	if len(agg) != 2 {
		t.Fatalf("expected 2 aggregates, got %d", len(agg))
	}

	// Should be sorted by SnapshotID ASC (snap-a first, then snap-b)
	if agg[0].SnapshotID != "snap-a" || agg[0].AggregateQuantity != 3 {
		t.Errorf("expected snap-a with qty 3, got %s with qty %d", agg[0].SnapshotID, agg[0].AggregateQuantity)
	}
	if agg[1].SnapshotID != "snap-b" || agg[1].AggregateQuantity != 6 {
		t.Errorf("expected snap-b with qty 6, got %s with qty %d", agg[1].SnapshotID, agg[1].AggregateQuantity)
	}
}
