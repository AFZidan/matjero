# P5.1 Customer + Cart Core Domain Implementation Report

## Delivery

- Base SHA: `c25150ada63de065d5d90990e056a91d5d0a56ca`
- Branch: `feature/p5-1-customer-cart-core-domain`
- Final head SHA: recorded in the delivery handoff after the final commit
- Scope: P5.1 Customer + Cart Core Domain only

## Implemented scope

- Migration `000010_customer_cart_domain` adds seller-owned fulfillment locations, customers, customer addresses, carts, and cart items. It preserves Supplier ownership, Market isolation, composite Store/Market and Customer/Store foreign keys, ownership checks, status/quantity/price constraints, and the required partial uniqueness rules.
- Customer and Cart persistence lives in `pkg/commerce/customer_cart.go`. Cart capabilities are generated with the existing cryptographic standard, only the digest is persisted, and the raw token is returned only at Cart creation.
- Cart Add, quantity update, and removal all lock the parent Cart row first with `FOR UPDATE`; checked-out carts are immutable. No merge operation is introduced in this slice.
- `pkg/catalog/selection.go` is the shared canonical Listing primitive. It selects the newest eligible Listing by `created_at DESC, id DESC` and is reused by Storefront catalog reads, Product/SKU availability, and Core Add-to-Cart resolution.
- Public Add-to-Cart accepts only `sku_id` and `quantity` plus the Cart capability. Core resolves SKU → Variant → Product → canonical Listing and snapshots the authoritative retail amount and currency. Listing/source/store/price input is not authority.
- Product and SKU availability is source-aware and uses the same canonical Listing: Supplier inventory is restricted to the Listing Supplier, seller-owned inventory to the Listing Store, active locations only, and explicit Supplier Offer unavailability is excluded. `inventory_snapshots` remain stock authority.
- Internal Core contracts and generated OpenAPI cover Cart create/read/add/update/remove and seller-only Store-owned location creation. Safe Cart responses omit Listing, Supplier, Offer, and Fulfillment Location identity.

## Tests and matrix coverage

Added deterministic database/integration coverage for Customer and Cart Store/Market isolation, address and Cart ownership, required price snapshots, quantity rules, fulfillment ownership branches, canonical newest Listing resolution, authoritative Add-to-Cart pricing, Cart capability digest storage, checked-out immutability, and source-aware inventory ownership. The tests map to the P5.1 cases 128–136 and the required cross-Store, cross-Supplier, inactive-location, and unavailable-Offer availability cases. No sleep-based race tests are used.

P5.2 and later checkout, Order, reservation, payment, shipping, and Customer IAM functionality were not started.

## Validation

The final handoff records the exact results for:

- `git diff --check`
- `GOWORK=off go mod tidy` and unchanged `go.mod`/`go.sum`
- `GOWORK=off go build ./...`
- `GOWORK=off go vet ./...`
- `GOWORK=off go test -count=1 ./...`
- `GOWORK=off go list -m all`
- `GOWORK=off go run ./cmd/openapi-gen` and generated OpenAPI drift validation
- `make migrate-check`
- `docker compose config --quiet`
- explicit `core-api`, `general-worker`, and `scheduler` builds
- local security check status and remote CI security status

Known limitation: local `gitleaks` is unavailable; the CI security job remains required. The generated OpenAPI file is intentionally included because P5.1 adds internal Core routes.
