package commerce

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/matjeroapps/core/internal/testdb"
	"github.com/matjeroapps/core/packages/database"
)

func applyAllMigrations(t *testing.T, db *database.Pool) {
	t.Helper()
	applySQLFile(t, db, filepath.Join("..", "..", "migrations", "000001_event_delivery_foundation.up.sql"))
	applySQLFile(t, db, filepath.Join("..", "..", "migrations", "000002_market_reference_data.up.sql"))
	applySQLFile(t, db, filepath.Join("..", "..", "migrations", "000003_commerce_domain_schema.up.sql"))
	applySQLFile(t, db, filepath.Join("..", "..", "migrations", "000004_admin_supplier_seller_platforms.up.sql"))
	applySQLFile(t, db, filepath.Join("..", "..", "migrations", "000005_store_domain_lifecycle.up.sql"))
	applySQLFile(t, db, filepath.Join("..", "..", "migrations", "000006_store_domain_integrity.up.sql"))
	applySQLFile(t, db, filepath.Join("..", "..", "migrations", "000007_theme_engine_schema.up.sql"))
	applySQLFile(t, db, filepath.Join("..", "..", "migrations", "000008_storefront_revisions.up.sql"))
	applySQLFile(t, db, filepath.Join("..", "..", "migrations", "000009_supplier_retail_capability.up.sql"))
}

func openSupplierRetailTestDB(t *testing.T) (*database.Pool, Repository, Service) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://commerce:commerce@localhost:5432/commerce?sslmode=disable"
	}
	db := testdb.Open(t, dsn)
	applyAllMigrations(t, db)
	repo := NewRepository(db.Pool)
	svc := NewService(repo)
	svc.PlatformDomain = "matjero.com"
	return db, repo, svc
}

func TestMigration000009_UpAndDown(t *testing.T) {
	db, repo, _ := openSupplierRetailTestDB(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	// Create test suppliers and sellers
	supA, err := repo.CreateSupplier(ctx, "sup-m9-a-"+suffix, "Supplier M9 A", "active", nil)
	if err != nil {
		t.Fatalf("CreateSupplier A: %v", err)
	}
	supB, err := repo.CreateSupplier(ctx, "sup-m9-b-"+suffix, "Supplier M9 B", "active", nil)
	if err != nil {
		t.Fatalf("CreateSupplier B: %v", err)
	}
	selA, err := repo.CreateSeller(ctx, "sel-m9-a-"+suffix, "Seller M9 A", "active", nil)
	if err != nil {
		t.Fatalf("CreateSeller A: %v", err)
	}
	selB, err := repo.CreateSeller(ctx, "sel-m9-b-"+suffix, "Seller M9 B", "active", nil)
	if err != nil {
		t.Fatalf("CreateSeller B: %v", err)
	}

	// 1. Supplier A <-> Seller A (PASS)
	affA, err := repo.CreateSupplierSellerAffiliation(ctx, supA.ID, selA.ID)
	if err != nil {
		t.Fatalf("CreateSupplierSellerAffiliation A: %v", err)
	}
	if affA.SupplierID != supA.ID || affA.SellerID != selA.ID {
		t.Fatalf("unexpected affiliation record: %+v", affA)
	}

	// 2. Supplier A <-> Seller B (FAIL unique supplier_id)
	_, err = repo.CreateSupplierSellerAffiliation(ctx, supA.ID, selB.ID)
	if err == nil || !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict for duplicate supplier affiliation, got %v", err)
	}

	// 3. Supplier B <-> Seller A (FAIL unique seller_id)
	_, err = repo.CreateSupplierSellerAffiliation(ctx, supB.ID, selA.ID)
	if err == nil || !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict for duplicate seller affiliation, got %v", err)
	}

	// 4. Down and Up cycle
	applySQLFile(t, db, filepath.Join("..", "..", "migrations", "000009_supplier_retail_capability.down.sql"))
	applySQLFile(t, db, filepath.Join("..", "..", "migrations", "000009_supplier_retail_capability.up.sql"))

	// Verify we can re-affiliate after clean migration cycle
	_, err = repo.CreateSupplierSellerAffiliation(ctx, supA.ID, selA.ID)
	if err != nil {
		t.Fatalf("re-affiliation after down/up cycle failed: %v", err)
	}
}

func TestSupplierRetailCapability_AtomicProvisioning(t *testing.T) {
	_, repo, svc := openSupplierRetailTestDB(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	subject := "sub-owner-" + suffix

	supplier, err := repo.CreateSupplier(ctx, "sup-prov-"+suffix, "Provisioning Supplier", "active", nil)
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	if _, err := repo.CreateSupplierMember(ctx, supplier.ID, subject, "owner", "active"); err != nil {
		t.Fatalf("CreateSupplierMember: %v", err)
	}

	draft := RetailCapabilityDraft{
		Code:     "ret-sel-" + suffix,
		Name:     "Retail Store Profile",
		Settings: map[string]any{"default_currency": "EGP"},
	}

	seller, affiliation, err := svc.CreateSupplierRetailCapabilityForSubject(ctx, subject, supplier.ID, draft)
	if err != nil {
		t.Fatalf("CreateSupplierRetailCapabilityForSubject: %v", err)
	}

	if seller.Code != draft.Code || seller.Name != draft.Name || seller.Status != "active" {
		t.Fatalf("unexpected seller state: %+v", seller)
	}
	if affiliation.SupplierID != supplier.ID || affiliation.SellerID != seller.ID {
		t.Fatalf("unexpected affiliation state: %+v", affiliation)
	}

	// Verify seller_settings
	settings, err := repo.GetSellerSettings(ctx, seller.ID)
	if err != nil {
		t.Fatalf("GetSellerSettings: %v", err)
	}
	if settings["default_currency"] != "EGP" {
		t.Fatalf("unexpected settings: %v", settings)
	}

	// Verify seller_members (initiating subject is active owner)
	sellerForSubject, err := repo.GetSellerForSubject(ctx, subject)
	if err != nil {
		t.Fatalf("GetSellerForSubject: %v", err)
	}
	if sellerForSubject.ID != seller.ID {
		t.Fatalf("resolved seller ID mismatch: got %s, want %s", sellerForSubject.ID, seller.ID)
	}

	// Second creation attempt should conflict
	_, _, err = svc.CreateSupplierRetailCapabilityForSubject(ctx, subject, supplier.ID, draft)
	if err == nil || !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict on duplicate capability creation, got %v", err)
	}
}

func TestSupplierRetailCapability_MembershipIsolation(t *testing.T) {
	_, repo, svc := openSupplierRetailTestDB(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	subjectOwner := "sub-owner-iso-" + suffix
	subjectManager := "sub-manager-iso-" + suffix

	supplier, err := repo.CreateSupplier(ctx, "sup-iso-"+suffix, "Isolated Supplier", "active", nil)
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	if _, err := repo.CreateSupplierMember(ctx, supplier.ID, subjectOwner, "owner", "active"); err != nil {
		t.Fatalf("CreateSupplierMember Owner: %v", err)
	}
	if _, err := repo.CreateSupplierMember(ctx, supplier.ID, subjectManager, "manager", "active"); err != nil {
		t.Fatalf("CreateSupplierMember Manager: %v", err)
	}

	draft := RetailCapabilityDraft{
		Code: "ret-iso-" + suffix,
		Name: "Isolated Retail Seller",
	}

	seller, _, err := svc.CreateSupplierRetailCapabilityForSubject(ctx, subjectOwner, supplier.ID, draft)
	if err != nil {
		t.Fatalf("CreateSupplierRetailCapabilityForSubject: %v", err)
	}

	// Owner subject resolves seller
	selOwner, err := repo.GetSellerForSubject(ctx, subjectOwner)
	if err != nil || selOwner.ID != seller.ID {
		t.Fatalf("owner subject failed to resolve seller: err=%v", err)
	}

	// Manager subject MUST NOT have seller membership copied
	_, err = repo.GetSellerForSubject(ctx, subjectManager)
	if err == nil || !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for manager subject on seller profile, got %v", err)
	}
}

func TestSupplierRetailCapability_StoreOwnership(t *testing.T) {
	_, repo, svc := openSupplierRetailTestDB(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	subject := "sub-store-owner-" + suffix

	supplier, err := repo.CreateSupplier(ctx, "sup-store-"+suffix, "Store Supplier", "active", nil)
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	if _, err := repo.CreateSupplierMember(ctx, supplier.ID, subject, "owner", "active"); err != nil {
		t.Fatalf("CreateSupplierMember: %v", err)
	}

	seller, _, err := svc.CreateSupplierRetailCapabilityForSubject(ctx, subject, supplier.ID, RetailCapabilityDraft{
		Code: "sel-store-" + suffix,
		Name: "Store Retail Seller",
	})
	if err != nil {
		t.Fatalf("CreateSupplierRetailCapabilityForSubject: %v", err)
	}

	// Create store under supplier retail flow
	storeCode := "supstore" + suffix
	store, err := svc.CreateSupplierStoreForSubject(ctx, subject, supplier.ID, "EG", storeCode, "Supplier Retail Store", "active", nil)
	if err != nil {
		t.Fatalf("CreateSupplierStoreForSubject: %v", err)
	}

	// Invariant Check: store.SellerID MUST equal seller.ID
	if store.SellerID != seller.ID {
		t.Fatalf("store seller_id mismatch: got %s, want %s", store.SellerID, seller.ID)
	}

	// Verify Store Access primitive
	fetchedStore, fetchedSeller, err := svc.RequireSupplierRetailStoreAccess(ctx, subject, supplier.ID, store.ID)
	if err != nil {
		t.Fatalf("RequireSupplierRetailStoreAccess: %v", err)
	}
	if fetchedStore.ID != store.ID || fetchedSeller.ID != seller.ID {
		t.Fatalf("RequireSupplierRetailStoreAccess returned mismatch: store=%s seller=%s", fetchedStore.ID, fetchedSeller.ID)
	}

	// Verify unauthorized / mismatched store access
	_, _, err = svc.RequireSupplierRetailStoreAccess(ctx, subject, supplier.ID, "00000000-0000-0000-0000-000000000000")
	if err == nil || !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for wrong store ID, got %v", err)
	}
}

func TestSupplierRetailCapability_SourceIntegrity(t *testing.T) {
	_, repo, svc := openSupplierRetailTestDB(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	// Setup Seller & Store
	seller, err := repo.CreateSeller(ctx, "sel-src-"+suffix, "Source Seller", "active", nil)
	if err != nil {
		t.Fatalf("CreateSeller: %v", err)
	}
	store, err := repo.CreateStore(ctx, seller.ID, "EG", "str-src-"+suffix, "Source Store", "active", nil)
	if err != nil {
		t.Fatalf("CreateStore: %v", err)
	}

	// Setup Products A & B
	prodA, err := repo.CreateProduct(ctx, "prod-a-"+suffix, "active")
	if err != nil {
		t.Fatalf("CreateProduct A: %v", err)
	}
	prodB, err := repo.CreateProduct(ctx, "prod-b-"+suffix, "active")
	if err != nil {
		t.Fatalf("CreateProduct B: %v", err)
	}

	// Setup Supplier & Offer A for Product A
	supplier, err := repo.CreateSupplier(ctx, "sup-src-"+suffix, "Source Supplier", "active", nil)
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	supMarket, err := repo.CreateSupplierMarket(ctx, supplier.ID, "EG", "active", nil)
	if err != nil {
		t.Fatalf("CreateSupplierMarket: %v", err)
	}
	supProdA, err := repo.CreateSupplierProduct(ctx, supplier.ID, prodA.ID, "SUP-PROD-A", "active")
	if err != nil {
		t.Fatalf("CreateSupplierProduct A: %v", err)
	}
	offerA, err := repo.CreateSupplierOffer(ctx, supplier.ID, supProdA.ID, supMarket.ID, "EG", "active")
	if err != nil {
		t.Fatalf("CreateSupplierOffer A: %v", err)
	}

	// Attempt SellerListing with Product B but Offer A (for Product A) -> MUST FAIL
	offerAID := offerA.ID
	_, err = svc.CreateSellerListing(ctx, store.ID, prodB.ID, &offerAID, "EG", "active")
	if err == nil || !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for mismatched offer product, got %v", err)
	}

	// Correct SellerListing with Product A + Offer A -> MUST SUCCEED
	listingA, err := svc.CreateSellerListing(ctx, store.ID, prodA.ID, &offerAID, "EG", "active")
	if err != nil {
		t.Fatalf("CreateSellerListing matching product & offer: %v", err)
	}
	if listingA.ProductID != prodA.ID || *listingA.SupplierOfferID != offerA.ID {
		t.Fatalf("unexpected listing state: %+v", listingA)
	}
}

func TestSupplierRetailCapability_SourceDerivation_OwnAndNetwork(t *testing.T) {
	_, repo, svc := openSupplierRetailTestDB(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	subjectA := "sub-owner-a-" + suffix

	// Supplier A with Retail capability -> Seller A
	supA, err := repo.CreateSupplier(ctx, "sup-der-a-"+suffix, "Supplier A", "active", nil)
	if err != nil {
		t.Fatalf("CreateSupplier A: %v", err)
	}
	if _, err := repo.CreateSupplierMember(ctx, supA.ID, subjectA, "owner", "active"); err != nil {
		t.Fatalf("CreateSupplierMember A: %v", err)
	}
	selA, affA, err := svc.CreateSupplierRetailCapabilityForSubject(ctx, subjectA, supA.ID, RetailCapabilityDraft{
		Code: "sel-der-a-" + suffix,
		Name: "Seller A",
	})
	if err != nil {
		t.Fatalf("CreateSupplierRetailCapabilityForSubject A: %v", err)
	}

	// Supplier B (separate wholesale supplier, no retail capability)
	supB, err := repo.CreateSupplier(ctx, "sup-der-b-"+suffix, "Supplier B", "active", nil)
	if err != nil {
		t.Fatalf("CreateSupplier B: %v", err)
	}

	// Setup Products & Offers
	prodA, _ := repo.CreateProduct(ctx, "prod-der-a-"+suffix, "active")
	supMktA, _ := repo.CreateSupplierMarket(ctx, supA.ID, "EG", "active", nil)
	supProdA, _ := repo.CreateSupplierProduct(ctx, supA.ID, prodA.ID, "SPA", "active")
	offerA, _ := repo.CreateSupplierOffer(ctx, supA.ID, supProdA.ID, supMktA.ID, "EG", "active")

	prodB, _ := repo.CreateProduct(ctx, "prod-der-b-"+suffix, "active")
	supMktB, _ := repo.CreateSupplierMarket(ctx, supB.ID, "EG", "active", nil)
	supProdB, _ := repo.CreateSupplierProduct(ctx, supB.ID, prodB.ID, "SPB", "active")
	offerB, _ := repo.CreateSupplierOffer(ctx, supB.ID, supProdB.ID, supMktB.ID, "EG", "active")

	// Store for Seller A
	storeA, err := repo.CreateStore(ctx, selA.ID, "EG", "str-der-"+suffix, "Store A", "active", nil)
	if err != nil {
		t.Fatalf("CreateStore: %v", err)
	}

	// Create listings
	offAID := offerA.ID
	listingOwn, err := repo.CreateSellerListing(ctx, storeA.ID, prodA.ID, &offAID, "EG", "active")
	if err != nil {
		t.Fatalf("CreateSellerListing Own: %v", err)
	}

	offBID := offerB.ID
	listingNet, err := repo.CreateSellerListing(ctx, storeA.ID, prodB.ID, &offBID, "EG", "active")
	if err != nil {
		t.Fatalf("CreateSellerListing Network: %v", err)
	}

	// Derive OWN vs NETWORK source semantics
	// Listing Own: offer -> supA.ID == affA.SupplierID (OWN)
	fetchedOfferOwn, err := repo.GetSupplierOffer(ctx, *listingOwn.SupplierOfferID)
	if err != nil {
		t.Fatalf("GetSupplierOffer Own: %v", err)
	}
	isOwn := fetchedOfferOwn.SupplierID == affA.SupplierID
	if !isOwn {
		t.Fatalf("expected listingOwn source to be OWN (supplier %s == affiliated %s)", fetchedOfferOwn.SupplierID, affA.SupplierID)
	}

	// Listing Net: offer -> supB.ID != affA.SupplierID (NETWORK)
	fetchedOfferNet, err := repo.GetSupplierOffer(ctx, *listingNet.SupplierOfferID)
	if err != nil {
		t.Fatalf("GetSupplierOffer Net: %v", err)
	}
	isNet := fetchedOfferNet.SupplierID != affA.SupplierID
	if !isNet {
		t.Fatalf("expected listingNet source to be NETWORK (supplier %s != affiliated %s)", fetchedOfferNet.SupplierID, affA.SupplierID)
	}
}
