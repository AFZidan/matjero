package themes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"dropshipping/internal/commerce"
	"dropshipping/internal/testdb"
	"dropshipping/packages/database"
)

type themesTestEnv struct {
	ctx     context.Context
	pool    *database.Pool
	repo    Repository
	svc     Service
	sellerA string
	sellerB string
	storeA  string
	storeB  string
}

func applyMigration(t *testing.T, db *database.Pool, name string) {
	t.Helper()
	sqlBytes, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	if _, err := db.Exec(context.Background(), string(sqlBytes)); err != nil {
		t.Fatalf("apply migration %s: %v", name, err)
	}
}

func setupThemesTest(t *testing.T) themesTestEnv {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://commerce:commerce@localhost:5432/commerce?sslmode=disable"
	}
	ctx := context.Background()
	db := testdb.Open(t, dsn)

	for _, m := range []string{
		"000002_market_reference_data.up.sql",
		"000003_commerce_domain_schema.up.sql",
		"000004_admin_supplier_seller_platforms.up.sql",
		"000005_store_domain_lifecycle.up.sql",
		"000006_store_domain_integrity.up.sql",
		"000007_theme_engine_schema.up.sql",
	} {
		applyMigration(t, db, m)
	}

	sellerA := uuid.NewString()
	sellerB := uuid.NewString()
	storeA := uuid.NewString()
	storeB := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO sellers (id, code, name, status) VALUES ($1,'seller-a','Seller A','active'),($2,'seller-b','Seller B','active')`, sellerA, sellerB); err != nil {
		t.Fatalf("insert sellers: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO stores (id, seller_id, market_code, code, name, status)
		VALUES ($1,$2,'EG','store-a','Store A','active'),($3,$4,'EG','store-b','Store B','active')`,
		storeA, sellerA, storeB, sellerB); err != nil {
		t.Fatalf("insert stores: %v", err)
	}

	repo := NewRepository(db.Pool)
	commerceRepo := commerce.NewRepository(db.Pool)
	svc := NewService(repo, commerceRepo, Options{PreviewSecret: []byte("test-secret")})
	if err := svc.SeedBuiltInThemes(ctx); err != nil {
		t.Fatalf("seed built-in themes: %v", err)
	}
	return themesTestEnv{ctx: ctx, pool: db, repo: repo, svc: svc, sellerA: sellerA, sellerB: sellerB, storeA: storeA, storeB: storeB}
}

func (e themesTestEnv) defaultThemeVersion(t *testing.T) ThemeVersion {
	t.Helper()
	theme, err := e.repo.GetThemeByKey(e.ctx, DefaultThemeKey)
	if err != nil {
		t.Fatalf("get default theme: %v", err)
	}
	v, err := e.repo.GetLatestPublishedVersion(e.ctx, theme.ID)
	if err != nil {
		t.Fatalf("get default version: %v", err)
	}
	return v
}

func TestThemePersistence(t *testing.T) {
	e := setupThemesTest(t)
	theme, err := e.repo.CreateTheme(e.ctx, "custom-1", "Custom One", "desc", ThemeTypeFree, ThemeStatusActive)
	if err != nil {
		t.Fatalf("CreateTheme: %v", err)
	}
	got, err := e.repo.GetTheme(e.ctx, theme.ID)
	if err != nil {
		t.Fatalf("GetTheme: %v", err)
	}
	if got.Key != "custom-1" || got.Type != ThemeTypeFree {
		t.Fatalf("unexpected theme: %+v", got)
	}
	themes, err := e.repo.ListThemes(e.ctx)
	if err != nil {
		t.Fatalf("ListThemes: %v", err)
	}
	if len(themes) < 2 { // default + custom
		t.Fatalf("expected at least 2 themes, got %d", len(themes))
	}
}

func TestVersionUniqueness(t *testing.T) {
	e := setupThemesTest(t)
	theme, err := e.repo.CreateTheme(e.ctx, "custom-2", "Custom Two", "desc", ThemeTypeFree, ThemeStatusActive)
	if err != nil {
		t.Fatalf("CreateTheme: %v", err)
	}
	if _, err := e.repo.CreateThemeVersion(e.ctx, theme.ID, "1.0.0", ThemeVersionStatusPublished, DefaultConfigurationSchema, DefaultConfiguration, "1.0.0"); err != nil {
		t.Fatalf("first version: %v", err)
	}
	if _, err := e.repo.CreateThemeVersion(e.ctx, theme.ID, "1.0.0", ThemeVersionStatusPublished, DefaultConfigurationSchema, DefaultConfiguration, "1.0.0"); err == nil {
		t.Fatal("expected duplicate (theme, version) to conflict")
	} else if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestOneActiveInstallationPerStore(t *testing.T) {
	e := setupThemesTest(t)
	v := e.defaultThemeVersion(t)
	if _, err := e.repo.CreateInstallation(e.ctx, e.storeA, v.ThemeID, v.ID, ThemeInstallationStatusActive); err != nil {
		t.Fatalf("first install: %v", err)
	}
	// second active installation for same store must deactivate the first
	if err := e.repo.DeactivateInstallations(e.ctx, e.storeA); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	v2, err := e.repo.CreateThemeVersion(e.ctx, v.ThemeID, "2.0.0", ThemeVersionStatusPublished, DefaultConfigurationSchema, DefaultConfiguration, "1.0.0")
	if err != nil {
		t.Fatalf("second version: %v", err)
	}
	if _, err := e.repo.CreateInstallation(e.ctx, e.storeA, v2.ThemeID, v2.ID, ThemeInstallationStatusActive); err != nil {
		t.Fatalf("second install: %v", err)
	}
	var activeCount int
	if err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM theme_installations WHERE store_id=$1 AND status='active'`, e.storeA).Scan(&activeCount); err != nil {
		t.Fatalf("count active: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("expected exactly 1 active installation, got %d", activeCount)
	}
}

func TestConfigurationPersistenceAndRevisions(t *testing.T) {
	e := setupThemesTest(t)
	v := e.defaultThemeVersion(t)
	inst, err := e.repo.CreateInstallation(e.ctx, e.storeA, v.ThemeID, v.ID, ThemeInstallationStatusActive)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := e.repo.CreateConfiguration(e.ctx, inst.ID, v.DefaultConfiguration, v.DefaultConfiguration); err != nil {
		t.Fatalf("create config: %v", err)
	}
	draft := map[string]any{"hero": map[string]any{"title": "New"}}
	rev, err := e.repo.UpdateDraftConfiguration(e.ctx, inst.ID, draft)
	if err != nil {
		t.Fatalf("update draft: %v", err)
	}
	if rev != 1 {
		t.Fatalf("expected draft revision 1, got %d", rev)
	}
	cfg, err := e.repo.GetConfiguration(e.ctx, inst.ID)
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if cfg.DraftRevision != 1 || cfg.PublishedRevision != 0 {
		t.Fatalf("unexpected revisions: draft=%d published=%d", cfg.DraftRevision, cfg.PublishedRevision)
	}
}

func TestPublishCopiesDraftAndBumpsRevision(t *testing.T) {
	e := setupThemesTest(t)
	if _, err := e.svc.Install(e.ctx, e.sellerA, e.storeA, DefaultThemeKey, ""); err != nil {
		t.Fatalf("install: %v", err)
	}
	newDraft := map[string]any{"hero": map[string]any{"title": "Published Title"}}
	if _, err := e.svc.UpdateDraftConfiguration(e.ctx, e.sellerA, e.storeA, newDraft); err != nil {
		t.Fatalf("update draft: %v", err)
	}
	pubRev, err := e.svc.PublishConfiguration(e.ctx, e.sellerA, e.storeA)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if pubRev != 1 {
		t.Fatalf("expected published revision 1, got %d", pubRev)
	}
	inst, cfg, err := e.svc.GetInstallation(e.ctx, e.sellerA, e.storeA)
	if err != nil {
		t.Fatalf("get installation: %v", err)
	}
	if inst.ID == "" {
		t.Fatal("missing installation")
	}
	hero, _ := cfg.PublishedConfig["hero"].(map[string]any)
	if hero["title"] != "Published Title" {
		t.Fatalf("published config not updated: %+v", cfg.PublishedConfig)
	}
	if cfg.PublishedRevision != 1 {
		t.Fatalf("expected published revision 1, got %d", cfg.PublishedRevision)
	}
}

func TestPublishInvalidDraftDoesNotPartialPublish(t *testing.T) {
	e := setupThemesTest(t)
	if _, err := e.svc.Install(e.ctx, e.sellerA, e.storeA, DefaultThemeKey, ""); err != nil {
		t.Fatalf("install: %v", err)
	}
	// Write an invalid (unsafe) draft directly through the repository, bypassing
	// service validation, to exercise the atomic publish guard.
	inst, err := e.repo.GetInstallationByStore(e.ctx, e.storeA)
	if err != nil {
		t.Fatalf("get installation: %v", err)
	}
	unsafe := map[string]any{"hero": map[string]any{"title": "<script>alert(1)</script>"}}
	if _, err := e.repo.UpdateDraftConfiguration(e.ctx, inst.ID, unsafe); err != nil {
		t.Fatalf("seed unsafe draft: %v", err)
	}
	if _, err := e.svc.PublishConfiguration(e.ctx, e.sellerA, e.storeA); err == nil {
		t.Fatal("expected publish of unsafe draft to fail")
	}
	cfg, err := e.repo.GetConfiguration(e.ctx, inst.ID)
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if cfg.PublishedRevision != 0 {
		t.Fatalf("published revision should remain 0 after failed publish, got %d", cfg.PublishedRevision)
	}
}

func TestFKConstraints(t *testing.T) {
	e := setupThemesTest(t)
	v := e.defaultThemeVersion(t)
	if _, err := e.repo.CreateInstallation(e.ctx, "non-existent-store", v.ThemeID, v.ID, ThemeInstallationStatusActive); err == nil {
		t.Fatal("expected FK violation for invalid store_id")
	}
	if _, err := e.repo.CreateThemeVersion(e.ctx, "non-existent-theme", "1.0.0", ThemeVersionStatusPublished, DefaultConfigurationSchema, DefaultConfiguration, "1.0.0"); err == nil {
		t.Fatal("expected FK violation for invalid theme_id")
	}
}

func TestInstallSwitchAndUpgrade(t *testing.T) {
	e := setupThemesTest(t)
	if _, err := e.svc.Install(e.ctx, e.sellerA, e.storeA, DefaultThemeKey, ""); err != nil {
		t.Fatalf("install default: %v", err)
	}
	// Upgrade to a newer published version.
	theme, err := e.repo.GetThemeByKey(e.ctx, DefaultThemeKey)
	if err != nil {
		t.Fatalf("get theme: %v", err)
	}
	if _, err := e.repo.CreateThemeVersion(e.ctx, theme.ID, "2.0.0", ThemeVersionStatusPublished, DefaultConfigurationSchema, DefaultConfiguration, "1.0.0"); err != nil {
		t.Fatalf("create v2: %v", err)
	}
	if err := e.svc.UpgradeInstallation(e.ctx, e.sellerA, e.storeA, "2.0.0"); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	inst, err := e.repo.GetInstallationByStore(e.ctx, e.storeA)
	if err != nil {
		t.Fatalf("get installation: %v", err)
	}
	v2, err := e.repo.GetThemeVersion(e.ctx, inst.ThemeVersionID)
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	if v2.Version != "2.0.0" {
		t.Fatalf("expected version 2.0.0 after upgrade, got %s", v2.Version)
	}
}

func TestAuthorizationCrossStore(t *testing.T) {
	e := setupThemesTest(t)
	// Seller A installs on Store A successfully.
	if _, err := e.svc.Install(e.ctx, e.sellerA, e.storeA, DefaultThemeKey, ""); err != nil {
		t.Fatalf("seller A install store A: %v", err)
	}
	// Seller A must NOT manage Store B.
	if _, err := e.svc.Install(e.ctx, e.sellerA, e.storeB, DefaultThemeKey, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-store install, got %v", err)
	}
	if _, _, err := e.svc.GetInstallation(e.ctx, e.sellerA, e.storeB); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-store get, got %v", err)
	}
	if _, _, err := e.svc.GetDraftConfiguration(e.ctx, e.sellerA, e.storeB); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-store draft read, got %v", err)
	}
	if _, err := e.svc.PublishConfiguration(e.ctx, e.sellerA, e.storeB); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-store publish, got %v", err)
	}
	if _, err := e.svc.CreatePreviewToken(e.ctx, e.sellerA, e.storeB); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-store preview, got %v", err)
	}
}

func TestPreviewTokenLifecycle(t *testing.T) {
	e := setupThemesTest(t)
	if _, err := e.svc.Install(e.ctx, e.sellerA, e.storeA, DefaultThemeKey, ""); err != nil {
		t.Fatalf("install: %v", err)
	}
	// Controllable clock for deterministic expiry.
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	svc := NewService(e.repo, commerce.NewRepository(e.pool.Pool), Options{PreviewSecret: []byte("test-secret"), Clock: clock})

	token, err := svc.CreatePreviewToken(e.ctx, e.sellerA, e.storeA)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	claims, err := svc.VerifyPreviewToken(token)
	if err != nil {
		t.Fatalf("verify valid token: %v", err)
	}
	if claims.StoreID != e.storeA {
		t.Fatalf("token store mismatch: %s", claims.StoreID)
	}

	// Expired token.
	now = now.Add(1 * time.Hour) // beyond 15m TTL
	if _, err := svc.VerifyPreviewToken(token); err == nil {
		t.Fatal("expected expired token to fail verification")
	}

	// Tampered signature.
	tampered := token + "x"
	if _, err := svc.VerifyPreviewToken(tampered); err == nil {
		t.Fatal("expected tampered token to fail verification")
	}

	// Wrong secret cannot verify.
	other := NewService(e.repo, commerce.NewRepository(e.pool.Pool), Options{PreviewSecret: []byte("other-secret"), Clock: clock})
	if _, err := other.VerifyPreviewToken(token); err == nil {
		t.Fatal("expected token signed with different secret to fail")
	}
}

func TestThemeMigrationUpDownReapply(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://commerce:commerce@localhost:5432/commerce?sslmode=disable"
	}
	db := testdb.Open(t, dsn)
	applyMigration(t, db, "000002_market_reference_data.up.sql")
	applyMigration(t, db, "000003_commerce_domain_schema.up.sql")
	applyMigration(t, db, "000007_theme_engine_schema.up.sql")
	// down
	downBytes, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000007_theme_engine_schema.down.sql"))
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	if _, err := db.Exec(context.Background(), string(downBytes)); err != nil {
		t.Fatalf("apply down migration: %v", err)
	}
	// up again (idempotent reapply)
	applyMigration(t, db, "000007_theme_engine_schema.up.sql")
}
