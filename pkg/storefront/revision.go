package storefront

import (
	"context"
	"errors"
	"fmt"
)

// Storefront cache revisions.
//
// A revision is the authoritative public cache generation of one store: an
// opaque, monotonically increasing counter that changes whenever anything the
// public storefront renders for that store changes. Core owns it because Core
// owns the commerce state it is derived from; consumers only read it.
//
// It exists so a downstream cache can be invalidated without deleting anything.
// A consumer that includes the revision in its cache key moves into a fresh
// namespace the moment the revision changes, and the entries it abandons expire
// on their own. There is no wildcard scan, no key registry, and no second event
// system.
//
// The revision resolves through the same tenant boundary as every other public
// read: a trusted host, then the store resolver, then the store. A store that no
// longer resolves publicly has no revision, which is what stops a consumer from
// serving cached content for a store that was suspended or disabled.

// RevisionStore reads the authoritative revision of a store.
// commerce.Repository satisfies it via StorefrontRevision.
type RevisionStore interface {
	StorefrontRevision(ctx context.Context, storeID string) (int64, error)
}

// RevisionReader resolves the public cache generation of a host-resolved store.
type RevisionReader struct {
	resolver StoreResolver
	store    RevisionStore
}

// NewRevisionReader builds a reader over a host resolver and a revision store.
func NewRevisionReader(resolver StoreResolver, store RevisionStore) RevisionReader {
	return RevisionReader{resolver: resolver, store: store}
}

// Revision returns the current public cache generation for a trusted host.
//
// An unknown host, an inactive domain and an inactive store all fail with the
// resolver's generic errors, so a consumer cannot tell them apart and cannot keep
// serving cached content for a store that stopped resolving publicly.
func (r RevisionReader) Revision(ctx context.Context, host string) (int64, error) {
	resolved, err := r.resolver.Resolve(ctx, host)
	if err != nil {
		return 0, err
	}
	revision, err := r.store.StorefrontRevision(ctx, resolved.Store.ID)
	if err != nil {
		return 0, fmt.Errorf("read storefront revision: %w", err)
	}
	return revision, nil
}

// RevisionFor returns the public cache generation of an already-resolved scope.
//
// Reads that already hold a scope use this so the catalog query and the revision
// it is labelled with describe the same store without resolving the host twice.
func (r RevisionReader) RevisionFor(ctx context.Context, scope CatalogScope) (int64, error) {
	if scope.storeID == "" {
		return 0, errors.New("storefront: revision requires a resolved scope")
	}
	revision, err := r.store.StorefrontRevision(ctx, scope.storeID)
	if err != nil {
		return 0, fmt.Errorf("read storefront revision: %w", err)
	}
	return revision, nil
}
