package commerce

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Public storefront cache revisions.
//
// A storefront revision is the authoritative cache generation of everything the
// public storefront renders for one store: bootstrap, published theme,
// categories, browse, search and product detail. It is opaque to consumers: it
// only ever answers "has the public output of this store changed".
//
// Every bump below runs on the transaction of the business write that caused it,
// so a committed change is never visible under its old generation and a
// rolled-back change never advances the generation. That is the whole
// invalidation mechanism: a consumer that keys its cache by revision never
// deletes anything, because a bump moves every later lookup into a new namespace
// and the abandoned entries expire on their own. No wildcard scan, no key
// registry, no second event system.
//
// A write is only bumped when it changes what the public storefront shows.
// Supplier wholesale prices, theme drafts, and seller/supplier/market status are
// deliberately not bumped: none of them appear in a public payload.

// listingStores resolves a mutated record to every store whose public output
// depends on it. This is what makes invalidation correct for records shared
// between stores: one supplier product listed by three stores bumps three
// generations, and a store that does not list it is untouched.
const listingStores = `SELECT sl.store_id FROM seller_listings sl`

// inventoryListingStores walks a stock row back to the stores that publish it.
// Public availability is derived from SKU inventory in active market fulfillment
// locations, so any movement along this chain can flip a product between in and
// out of stock.
const inventoryListingStores = `
	SELECT sl.store_id
	FROM inventory_snapshots inv
	JOIN skus sk ON sk.id = inv.sku_id
	JOIN variants v ON v.id = sk.variant_id
	JOIN seller_listings sl ON sl.product_id = v.product_id
	LEFT JOIN supplier_offers so ON so.id = sl.supplier_offer_id
	JOIN fulfillment_locations fl ON fl.id = inv.fulfillment_location_id
		AND (
			(sl.supplier_offer_id IS NULL AND fl.store_id = sl.store_id AND fl.supplier_id IS NULL)
			OR
			(sl.supplier_offer_id IS NOT NULL AND fl.store_id IS NULL AND fl.supplier_id = so.supplier_id)
		)
`

// Store selectors keyed by the identifier of the mutated record. They are
// trusted SQL declared here, never caller input; the identifier they compare
// against is always a bound parameter.
const (
	revisionStoreItself = `s.id = $1`

	revisionStoresByListing = `s.id IN (` + listingStores + ` WHERE sl.id = $1)`

	revisionStoresByProduct = `s.id IN (` + listingStores + ` WHERE sl.product_id = $1)`

	revisionStoresBySupplierOffer = `s.id IN (` + listingStores + ` WHERE sl.supplier_offer_id = $1)`

	// A variant is public as a selectable option and through variant_count, so
	// adding or restyling one changes the product payload of every store listing
	// its product.
	revisionStoresByVariant = `s.id IN (
		SELECT sl.store_id
		FROM variants v
		JOIN seller_listings sl ON sl.product_id = v.product_id
		WHERE v.id = $1
	)`

	revisionStoresBySKU = `s.id IN (
		SELECT sl.store_id
		FROM skus sk
		JOIN variants v ON v.id = sk.variant_id
		JOIN seller_listings sl ON sl.product_id = v.product_id
		WHERE sk.id = $1
	)`

	revisionStoresByInventorySnapshot = `s.id IN (` + inventoryListingStores + ` WHERE inv.id = $1)`

	// A fulfillment location only contributes stock while it is active, so its
	// status change alters availability everywhere its inventory is counted.
	revisionStoresByFulfillmentLocation = `s.id IN (` + inventoryListingStores + ` WHERE inv.fulfillment_location_id = $1)`

	// The public category tree is assembled from a category and its descendants,
	// so a change anywhere in the subtree can add, remove or rename a public node.
	revisionStoresByCategorySubtree = `s.id IN (
		WITH RECURSIVE subtree (id) AS (
			SELECT $1::uuid
			UNION
			SELECT c.id FROM categories c JOIN subtree t ON t.id = c.parent_category_id
		)
		SELECT sl.store_id
		FROM subtree t
		JOIN product_categories pc ON pc.category_id = t.id
		JOIN seller_listings sl ON sl.product_id = pc.product_id
	)`
)

// bumpStorefrontRevisions advances the generation of every store selected by
// predicate, on an open transaction.
//
// The statement is an upsert rather than a plain UPDATE for two reasons. It is
// atomic and therefore correct under concurrent writes: PostgreSQL locks the
// conflicting row, so two simultaneous bumps advance the generation twice instead
// of racing on a read-modify-write. And it is self-healing: a store with no row
// reads as generation 1, so inserting 2 is exactly the generation that follows.
func bumpStorefrontRevisions(ctx context.Context, tx pgx.Tx, predicate string, id any) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO storefront_revisions (store_id, revision)
		SELECT s.id, 2
		FROM stores s
		WHERE `+predicate+`
		ON CONFLICT (store_id) DO UPDATE
		SET revision = storefront_revisions.revision + 1,
		    updated_at = now()
	`, id)
	return translatePGError(err, "bump storefront revision")
}

// initStorefrontRevision gives a new store its first generation, so a storefront
// never starts without one.
func initStorefrontRevision(ctx context.Context, tx pgx.Tx, storeID string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO storefront_revisions (store_id) VALUES ($1)
		ON CONFLICT (store_id) DO NOTHING
	`, storeID)
	return translatePGError(err, "initialize storefront revision")
}

// StorefrontRevision returns the authoritative public cache generation of a
// store.
//
// A store with no row is reported as generation 1, which is the value the
// migration backfills and the value the first bump advances from. A store
// therefore always has a usable, monotonic generation, and a read never writes.
func (r Repository) StorefrontRevision(ctx context.Context, storeID string) (int64, error) {
	if storeID == "" {
		return 0, ErrInvalidInput
	}
	var revision int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COALESCE((SELECT revision FROM storefront_revisions WHERE store_id = $1), 1)
	`, storeID).Scan(&revision); err != nil {
		return 0, translatePGError(err, "get storefront revision")
	}
	return revision, nil
}

// updateStorefrontStatus applies a status mutation and advances the storefront
// generation of every affected store in the same transaction.
//
// table and predicate are constants declared in this package, never caller
// input.
func (r Repository) updateStorefrontStatus(ctx context.Context, table, predicate, id, status string) error {
	if id == "" || status == "" {
		return ErrInvalidInput
	}
	return r.withTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET status = $2, updated_at = now() WHERE id = $1`, table), id, status); err != nil {
			return translatePGError(err, "update status")
		}
		return bumpStorefrontRevisions(ctx, tx, predicate, id)
	})
}
