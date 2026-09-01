package themes

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Public storefront cache revisions, from the Theme Engine side.
//
// The storefront bootstrap carries the store's published theme and published
// configuration, so a theme change is a public output change and must advance the
// store's cache generation. The authoritative table is owned by the commerce
// package, which bumps it for every commerce write; the Theme Engine keeps its
// own statement here rather than importing commerce, because the two packages are
// deliberately decoupled and each resolves the affected store differently.
//
// Draft edits, draft discards and preview tokens never bump: a draft is invisible
// to customers until it is published, and invalidating on every keystroke would
// throw away a store's whole public cache for nothing.

// Store selectors for theme writes. They are trusted SQL declared here, never
// caller input; the identifier compared against is always a bound parameter.
const (
	revisionStoreItself = `s.id = $1`

	revisionStoreByInstallation = `s.id IN (
		SELECT ti.store_id FROM theme_installations ti WHERE ti.id = $1
	)`
)

// bumpStorefrontRevision advances the public cache generation of the store
// selected by predicate, on the transaction of the theme write that caused it.
//
// The upsert is atomic, so concurrent publishes advance the generation twice
// instead of racing on a read-modify-write, and a store with no row yet is
// seeded at the generation that follows the implicit initial one.
func bumpStorefrontRevision(ctx context.Context, tx pgx.Tx, predicate string, id string) error {
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
