package themes

import (
	"context"
	"time"
)

// StoreLookup resolves the owning seller of a store. It is satisfied by
// commerce.Repository and keeps the themes package decoupled from commerce core.
type StoreLookup interface {
	StoreSellerID(ctx context.Context, storeID string) (string, error)
}

// Service implements the Theme Engine business logic: installation, draft editing,
// atomic publishing, version upgrades, and preview-token issuance. Every
// store-scoped operation enforces resource-level authorization via StoreLookup.
type Service struct {
	repo          Repository
	stores        StoreLookup
	previewSecret []byte
	previewTTL    time.Duration
	clock         func() time.Time
}

// Options configures the theme Service.
type Options struct {
	PreviewSecret []byte
	PreviewTTL    time.Duration
	// Clock returns the current time. It defaults to time.Now and is overridable
	// in tests to control preview-token expiry deterministically.
	Clock func() time.Time
}

// NewService constructs a theme Service. previewTTL defaults to 15 minutes when
// not set; the preview secret must be configured via Options (never hardcoded).
func NewService(repo Repository, stores StoreLookup, opts Options) Service {
	ttl := opts.PreviewTTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return Service{repo: repo, stores: stores, previewSecret: opts.PreviewSecret, previewTTL: ttl, clock: clock}
}

// authorizeStore verifies that the resolved seller owns the store. A store the
// seller does not own is reported as not-found (safe not-found policy), so it is
// indistinguishable from a non-existent store to the caller.
func (s Service) authorizeStore(ctx context.Context, sellerID, storeID string) error {
	ownerID, err := s.stores.StoreSellerID(ctx, storeID)
	if err != nil {
		return err
	}
	if ownerID != sellerID {
		return ErrNotFound
	}
	return nil
}

// ListThemes returns all registered themes (platform-controlled catalog).
func (s Service) ListThemes(ctx context.Context) ([]Theme, error) {
	return s.repo.ListThemes(ctx)
}

// GetThemeByKey returns a single theme by its stable unique key.
func (s Service) GetThemeByKey(ctx context.Context, key string) (Theme, error) {
	if key == "" {
		return Theme{}, ErrInvalidInput
	}
	return s.repo.GetThemeByKey(ctx, key)
}

// ListThemeVersions returns all versions for a theme.
func (s Service) ListThemeVersions(ctx context.Context, themeID string) ([]ThemeVersion, error) {
	if themeID == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.ListThemeVersions(ctx, themeID)
}

// GetInstallation returns the active installation and its configuration for a
// store, enforcing store ownership.
func (s Service) GetInstallation(ctx context.Context, sellerID, storeID string) (ThemeInstallation, ThemeConfiguration, error) {
	if err := s.authorizeStore(ctx, sellerID, storeID); err != nil {
		return ThemeInstallation{}, ThemeConfiguration{}, err
	}
	inst, err := s.repo.GetInstallationByStore(ctx, storeID)
	if err != nil {
		return ThemeInstallation{}, ThemeConfiguration{}, err
	}
	cfg, err := s.repo.GetConfiguration(ctx, inst.ID)
	if err != nil {
		return ThemeInstallation{}, ThemeConfiguration{}, err
	}
	return inst, cfg, nil
}

// Install binds a store to a theme version, creating an active installation and a
// configuration initialized from the version's default configuration (both draft
// and published). If the store already has an active installation it is
// deactivated (switching themes never mutates commerce data). When version is
// empty the latest published version is used.
func (s Service) Install(ctx context.Context, sellerID, storeID, themeKey, version string) (ThemeInstallation, error) {
	if err := s.authorizeStore(ctx, sellerID, storeID); err != nil {
		return ThemeInstallation{}, err
	}
	theme, err := s.repo.GetThemeByKey(ctx, themeKey)
	if err != nil {
		return ThemeInstallation{}, err
	}
	if theme.Status != ThemeStatusActive {
		return ThemeInstallation{}, ErrInvalidInput
	}
	var v ThemeVersion
	if version == "" {
		v, err = s.repo.GetLatestPublishedVersion(ctx, theme.ID)
	} else {
		v, err = s.repo.GetThemeVersionByThemeAndVersion(ctx, theme.ID, version)
	}
	if err != nil {
		return ThemeInstallation{}, err
	}
	if v.Status != ThemeVersionStatusPublished {
		return ThemeInstallation{}, ErrInvalidInput
	}
	if err := s.repo.DeactivateInstallations(ctx, storeID); err != nil {
		return ThemeInstallation{}, err
	}
	inst, err := s.repo.CreateInstallation(ctx, storeID, theme.ID, v.ID, ThemeInstallationStatusActive)
	if err != nil {
		return ThemeInstallation{}, err
	}
	if _, err := s.repo.CreateConfiguration(ctx, inst.ID, v.DefaultConfiguration, v.DefaultConfiguration); err != nil {
		return ThemeInstallation{}, err
	}
	return inst, nil
}

// GetDraftConfiguration returns the draft config and its revision for a store.
func (s Service) GetDraftConfiguration(ctx context.Context, sellerID, storeID string) (map[string]any, int, error) {
	if err := s.authorizeStore(ctx, sellerID, storeID); err != nil {
		return nil, 0, err
	}
	inst, err := s.repo.GetInstallationByStore(ctx, storeID)
	if err != nil {
		return nil, 0, err
	}
	cfg, err := s.repo.GetConfiguration(ctx, inst.ID)
	if err != nil {
		return nil, 0, err
	}
	return cfg.DraftConfig, cfg.DraftRevision, nil
}

// UpdateDraftConfiguration validates the new config against the theme version
// schema and rejects unsafe content, then writes it as the new draft and bumps
// the draft revision. The live published config is untouched.
func (s Service) UpdateDraftConfiguration(ctx context.Context, sellerID, storeID string, config map[string]any) (int, error) {
	if err := s.authorizeStore(ctx, sellerID, storeID); err != nil {
		return 0, err
	}
	inst, err := s.repo.GetInstallationByStore(ctx, storeID)
	if err != nil {
		return 0, err
	}
	v, err := s.repo.GetThemeVersion(ctx, inst.ThemeVersionID)
	if err != nil {
		return 0, err
	}
	if err := ValidateConfiguration(v.ConfigurationSchema, config); err != nil {
		return 0, err
	}
	if err := RejectUnsafeContent(config); err != nil {
		return 0, err
	}
	return s.repo.UpdateDraftConfiguration(ctx, inst.ID, config)
}

// PublishConfiguration validates the draft against the theme version schema, then
// atomically copies the draft into published and bumps the published revision. On
// any validation failure no partial publish occurs.
func (s Service) PublishConfiguration(ctx context.Context, sellerID, storeID string) (int, error) {
	if err := s.authorizeStore(ctx, sellerID, storeID); err != nil {
		return 0, err
	}
	inst, err := s.repo.GetInstallationByStore(ctx, storeID)
	if err != nil {
		return 0, err
	}
	v, err := s.repo.GetThemeVersion(ctx, inst.ThemeVersionID)
	if err != nil {
		return 0, err
	}
	cfg, err := s.repo.GetConfiguration(ctx, inst.ID)
	if err != nil {
		return 0, err
	}
	if err := ValidateConfiguration(v.ConfigurationSchema, cfg.DraftConfig); err != nil {
		return 0, err
	}
	if err := RejectUnsafeContent(cfg.DraftConfig); err != nil {
		return 0, err
	}
	published, err := s.repo.PublishConfiguration(ctx, inst.ID)
	if err != nil {
		return 0, err
	}
	return published.PublishedRevision, nil
}

// DiscardDraft resets the draft to the currently published config and bumps the
// draft revision.
func (s Service) DiscardDraft(ctx context.Context, sellerID, storeID string) (int, error) {
	if err := s.authorizeStore(ctx, sellerID, storeID); err != nil {
		return 0, err
	}
	inst, err := s.repo.GetInstallationByStore(ctx, storeID)
	if err != nil {
		return 0, err
	}
	return s.repo.DiscardDraft(ctx, inst.ID)
}

// UpgradeInstallation points an existing installation at a newer published theme
// version. It validates the current draft against the target schema and rejects
// the upgrade safely when incompatible (no automated config migration yet).
func (s Service) UpgradeInstallation(ctx context.Context, sellerID, storeID, targetVersion string) error {
	if err := s.authorizeStore(ctx, sellerID, storeID); err != nil {
		return err
	}
	inst, err := s.repo.GetInstallationByStore(ctx, storeID)
	if err != nil {
		return err
	}
	theme, err := s.repo.GetTheme(ctx, inst.ThemeID)
	if err != nil {
		return err
	}
	target, err := s.repo.GetThemeVersionByThemeAndVersion(ctx, theme.ID, targetVersion)
	if err != nil {
		return err
	}
	if target.Status != ThemeVersionStatusPublished {
		return ErrInvalidInput
	}
	if target.ID == inst.ThemeVersionID {
		return ErrConflict
	}
	cfg, err := s.repo.GetConfiguration(ctx, inst.ID)
	if err != nil {
		return err
	}
	if err := ValidateConfiguration(target.ConfigurationSchema, cfg.DraftConfig); err != nil {
		return err
	}
	if err := RejectUnsafeContent(cfg.DraftConfig); err != nil {
		return err
	}
	return s.repo.UpdateInstallationVersion(ctx, inst.ID, target.ID)
}

// CreatePreviewToken issues a signed, short-lived, store-scoped preview token for
// the current draft revision. It enforces store ownership.
func (s Service) CreatePreviewToken(ctx context.Context, sellerID, storeID string) (string, error) {
	if err := s.authorizeStore(ctx, sellerID, storeID); err != nil {
		return "", err
	}
	inst, err := s.repo.GetInstallationByStore(ctx, storeID)
	if err != nil {
		return "", err
	}
	cfg, err := s.repo.GetConfiguration(ctx, inst.ID)
	if err != nil {
		return "", err
	}
	return s.IssuePreviewToken(inst.StoreID, inst.ID, cfg.DraftRevision)
}
