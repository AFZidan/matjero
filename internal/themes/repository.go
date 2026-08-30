package themes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides persistence for the Theme Engine. It owns the theme domain
// tables and keeps theme logic separate from commerce core repositories.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a Theme Engine repository from a pgx pool.
func NewRepository(pool *pgxpool.Pool) Repository {
	return Repository{pool: pool}
}

// Pool exposes the underlying pool (used by tests and callers needing a tx).
func (r Repository) Pool() *pgxpool.Pool {
	return r.pool
}

func (r Repository) withTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func translatePGError(err error, action string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%w: %s", ErrConflict, action)
		case "23503", "23514": // foreign_key_violation, check_violation
			return fmt.Errorf("%w: %s", ErrInvalidInput, action)
		}
	}
	return fmt.Errorf("%s: %w", action, err)
}

// --- Themes ---

func (r Repository) CreateTheme(ctx context.Context, key, name, description, themeType, status string) (Theme, error) {
	if key == "" || name == "" || !ValidThemeType(themeType) || !ValidThemeStatus(status) {
		return Theme{}, ErrInvalidInput
	}
	var t Theme
	id := uuid.NewString()
	err := r.pool.QueryRow(ctx, `
		INSERT INTO themes (id, key, name, description, type, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, updated_at
	`, id, key, name, description, themeType, status).Scan(&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return Theme{}, translatePGError(err, "create theme")
	}
	t.ID, t.Key, t.Name, t.Description, t.Type, t.Status = id, key, name, description, themeType, status
	return t, nil
}

func (r Repository) GetTheme(ctx context.Context, id string) (Theme, error) {
	var t Theme
	err := r.pool.QueryRow(ctx, `
		SELECT id, key, name, description, type, status, created_at, updated_at
		FROM themes WHERE id = $1
	`, id).Scan(&t.ID, &t.Key, &t.Name, &t.Description, &t.Type, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return Theme{}, translatePGError(err, "get theme")
	}
	return t, nil
}

func (r Repository) GetThemeByKey(ctx context.Context, key string) (Theme, error) {
	var t Theme
	err := r.pool.QueryRow(ctx, `
		SELECT id, key, name, description, type, status, created_at, updated_at
		FROM themes WHERE key = $1
	`, key).Scan(&t.ID, &t.Key, &t.Name, &t.Description, &t.Type, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return Theme{}, translatePGError(err, "get theme by key")
	}
	return t, nil
}

func (r Repository) ListThemes(ctx context.Context) ([]Theme, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, key, name, description, type, status, created_at, updated_at
		FROM themes ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list themes: %w", err)
	}
	defer rows.Close()
	var out []Theme
	for rows.Next() {
		var t Theme
		if err := rows.Scan(&t.ID, &t.Key, &t.Name, &t.Description, &t.Type, &t.Status, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan theme: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// --- Theme Versions ---

func (r Repository) CreateThemeVersion(ctx context.Context, themeID, version, status string, schema, defaultConfig map[string]any, componentRegistryVersion string) (ThemeVersion, error) {
	if themeID == "" || version == "" || !ValidThemeVersionStatus(status) {
		return ThemeVersion{}, ErrInvalidInput
	}
	if schema == nil {
		schema = map[string]any{}
	}
	if defaultConfig == nil {
		defaultConfig = map[string]any{}
	}
	var v ThemeVersion
	id := uuid.NewString()
	schemaBytes, _ := json.Marshal(schema)
	defaultBytes, _ := json.Marshal(defaultConfig)
	var publishedAt *time.Time
	if status == ThemeVersionStatusPublished {
		now := time.Now()
		publishedAt = &now
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO theme_versions (id, theme_id, version, status, configuration_schema, default_configuration, component_registry_version, published_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at
	`, id, themeID, version, status, schemaBytes, defaultBytes, componentRegistryVersion, publishedAt).Scan(&v.CreatedAt)
	if err != nil {
		return ThemeVersion{}, translatePGError(err, "create theme version")
	}
	v.ID = id
	v.ThemeID = themeID
	v.Version = version
	v.Status = status
	v.ConfigurationSchema = schema
	v.DefaultConfiguration = defaultConfig
	v.ComponentRegistryVersion = componentRegistryVersion
	v.PublishedAt = publishedAt
	return v, nil
}

func (r Repository) GetThemeVersion(ctx context.Context, id string) (ThemeVersion, error) {
	var v ThemeVersion
	var schemaBytes, defaultBytes []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, theme_id, version, status, configuration_schema, default_configuration, component_registry_version, created_at, published_at, deprecated_at
		FROM theme_versions WHERE id = $1
	`, id).Scan(&v.ID, &v.ThemeID, &v.Version, &v.Status, &schemaBytes, &defaultBytes, &v.ComponentRegistryVersion, &v.CreatedAt, &v.PublishedAt, &v.DeprecatedAt)
	if err != nil {
		return ThemeVersion{}, translatePGError(err, "get theme version")
	}
	_ = json.Unmarshal(schemaBytes, &v.ConfigurationSchema)
	_ = json.Unmarshal(defaultBytes, &v.DefaultConfiguration)
	return v, nil
}

func (r Repository) GetThemeVersionByThemeAndVersion(ctx context.Context, themeID, version string) (ThemeVersion, error) {
	var v ThemeVersion
	var schemaBytes, defaultBytes []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, theme_id, version, status, configuration_schema, default_configuration, component_registry_version, created_at, published_at, deprecated_at
		FROM theme_versions WHERE theme_id = $1 AND version = $2
	`, themeID, version).Scan(&v.ID, &v.ThemeID, &v.Version, &v.Status, &schemaBytes, &defaultBytes, &v.ComponentRegistryVersion, &v.CreatedAt, &v.PublishedAt, &v.DeprecatedAt)
	if err != nil {
		return ThemeVersion{}, translatePGError(err, "get theme version by theme and version")
	}
	_ = json.Unmarshal(schemaBytes, &v.ConfigurationSchema)
	_ = json.Unmarshal(defaultBytes, &v.DefaultConfiguration)
	return v, nil
}

func (r Repository) GetLatestPublishedVersion(ctx context.Context, themeID string) (ThemeVersion, error) {
	var v ThemeVersion
	var schemaBytes, defaultBytes []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, theme_id, version, status, configuration_schema, default_configuration, component_registry_version, created_at, published_at, deprecated_at
		FROM theme_versions
		WHERE theme_id = $1 AND status = 'published'
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, themeID).Scan(&v.ID, &v.ThemeID, &v.Version, &v.Status, &schemaBytes, &defaultBytes, &v.ComponentRegistryVersion, &v.CreatedAt, &v.PublishedAt, &v.DeprecatedAt)
	if err != nil {
		return ThemeVersion{}, translatePGError(err, "get latest published version")
	}
	_ = json.Unmarshal(schemaBytes, &v.ConfigurationSchema)
	_ = json.Unmarshal(defaultBytes, &v.DefaultConfiguration)
	return v, nil
}

func (r Repository) ListThemeVersions(ctx context.Context, themeID string) ([]ThemeVersion, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, theme_id, version, status, configuration_schema, default_configuration, component_registry_version, created_at, published_at, deprecated_at
		FROM theme_versions WHERE theme_id = $1 ORDER BY created_at ASC, id ASC
	`, themeID)
	if err != nil {
		return nil, fmt.Errorf("list theme versions: %w", err)
	}
	defer rows.Close()
	var out []ThemeVersion
	for rows.Next() {
		var v ThemeVersion
		var schemaBytes, defaultBytes []byte
		if err := rows.Scan(&v.ID, &v.ThemeID, &v.Version, &v.Status, &schemaBytes, &defaultBytes, &v.ComponentRegistryVersion, &v.CreatedAt, &v.PublishedAt, &v.DeprecatedAt); err != nil {
			return nil, fmt.Errorf("scan theme version: %w", err)
		}
		_ = json.Unmarshal(schemaBytes, &v.ConfigurationSchema)
		_ = json.Unmarshal(defaultBytes, &v.DefaultConfiguration)
		out = append(out, v)
	}
	return out, rows.Err()
}

// --- Installations ---

func (r Repository) CreateInstallation(ctx context.Context, storeID, themeID, themeVersionID, status string) (ThemeInstallation, error) {
	if storeID == "" || themeID == "" || themeVersionID == "" || !ValidThemeInstallationStatus(status) {
		return ThemeInstallation{}, ErrInvalidInput
	}
	var inst ThemeInstallation
	id := uuid.NewString()
	err := r.pool.QueryRow(ctx, `
		INSERT INTO theme_installations (id, store_id, theme_id, theme_version_id, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING installed_at, created_at, updated_at
	`, id, storeID, themeID, themeVersionID, status).Scan(&inst.InstalledAt, &inst.CreatedAt, &inst.UpdatedAt)
	if err != nil {
		return ThemeInstallation{}, translatePGError(err, "create installation")
	}
	inst.ID, inst.StoreID, inst.ThemeID, inst.ThemeVersionID, inst.Status = id, storeID, themeID, themeVersionID, status
	return inst, nil
}

// GetInstallationByStore returns the single active installation for a store.
func (r Repository) GetInstallationByStore(ctx context.Context, storeID string) (ThemeInstallation, error) {
	var inst ThemeInstallation
	err := r.pool.QueryRow(ctx, `
		SELECT id, store_id, theme_id, theme_version_id, status, installed_at, created_at, updated_at
		FROM theme_installations WHERE store_id = $1 AND status = 'active'
	`, storeID).Scan(&inst.ID, &inst.StoreID, &inst.ThemeID, &inst.ThemeVersionID, &inst.Status, &inst.InstalledAt, &inst.CreatedAt, &inst.UpdatedAt)
	if err != nil {
		return ThemeInstallation{}, translatePGError(err, "get installation")
	}
	return inst, nil
}

// DeactivateInstallations flips any active installation for a store to inactive.
// The partial unique index guarantees at most one active installation remains.
func (r Repository) DeactivateInstallations(ctx context.Context, storeID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE theme_installations SET status = 'inactive', updated_at = now()
		WHERE store_id = $1 AND status = 'active'
	`, storeID)
	return translatePGError(err, "deactivate installations")
}

func (r Repository) UpdateInstallationVersion(ctx context.Context, installationID, themeVersionID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE theme_installations SET theme_version_id = $2, updated_at = now() WHERE id = $1
	`, installationID, themeVersionID)
	return translatePGError(err, "update installation version")
}

// --- Configurations ---

func (r Repository) CreateConfiguration(ctx context.Context, installationID string, draft, published map[string]any) (ThemeConfiguration, error) {
	if installationID == "" {
		return ThemeConfiguration{}, ErrInvalidInput
	}
	if draft == nil {
		draft = map[string]any{}
	}
	if published == nil {
		published = map[string]any{}
	}
	var c ThemeConfiguration
	id := uuid.NewString()
	draftBytes, _ := json.Marshal(draft)
	publishedBytes, _ := json.Marshal(published)
	err := r.pool.QueryRow(ctx, `
		INSERT INTO theme_configurations (id, installation_id, draft_config, published_config)
		VALUES ($1, $2, $3, $4)
		RETURNING updated_at
	`, id, installationID, draftBytes, publishedBytes).Scan(&c.UpdatedAt)
	if err != nil {
		return ThemeConfiguration{}, translatePGError(err, "create configuration")
	}
	c.ID = id
	c.InstallationID = installationID
	c.DraftConfig = draft
	c.PublishedConfig = published
	c.DraftRevision = 0
	c.PublishedRevision = 0
	return c, nil
}

func (r Repository) GetConfiguration(ctx context.Context, installationID string) (ThemeConfiguration, error) {
	var c ThemeConfiguration
	var draftBytes, publishedBytes []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, installation_id, draft_config, published_config, draft_revision, published_revision, updated_at, published_at
		FROM theme_configurations WHERE installation_id = $1
	`, installationID).Scan(&c.ID, &c.InstallationID, &draftBytes, &publishedBytes, &c.DraftRevision, &c.PublishedRevision, &c.UpdatedAt, &c.PublishedAt)
	if err != nil {
		return ThemeConfiguration{}, translatePGError(err, "get configuration")
	}
	_ = json.Unmarshal(draftBytes, &c.DraftConfig)
	_ = json.Unmarshal(publishedBytes, &c.PublishedConfig)
	return c, nil
}

func (r Repository) UpdateDraftConfiguration(ctx context.Context, installationID string, draft map[string]any) (int, error) {
	draftBytes, _ := json.Marshal(draft)
	var rev int
	err := r.pool.QueryRow(ctx, `
		UPDATE theme_configurations
		SET draft_config = $2, draft_revision = draft_revision + 1, updated_at = now()
		WHERE installation_id = $1
		RETURNING draft_revision
	`, installationID, draftBytes).Scan(&rev)
	if err != nil {
		return 0, translatePGError(err, "update draft configuration")
	}
	return rev, nil
}

// PublishConfiguration atomically copies the current draft into published and
// bumps the published revision inside a single transaction. The row is locked
// with FOR UPDATE so concurrent publishes cannot double-bump the revision.
func (r Repository) PublishConfiguration(ctx context.Context, installationID string) (ThemeConfiguration, error) {
	var c ThemeConfiguration
	var draftBytes, publishedBytes []byte
	err := r.withTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT draft_config FROM theme_configurations WHERE installation_id = $1 FOR UPDATE
		`, installationID).Scan(&draftBytes); err != nil {
			return translatePGError(err, "lock configuration")
		}
		now := time.Now()
		return tx.QueryRow(ctx, `
			UPDATE theme_configurations
			SET published_config = $2,
				published_revision = published_revision + 1,
				published_at = $3,
				updated_at = now()
			WHERE installation_id = $1
			RETURNING id, installation_id, draft_config, published_config, draft_revision, published_revision, updated_at, published_at
		`, installationID, draftBytes, now).Scan(
			&c.ID, &c.InstallationID, &draftBytes, &publishedBytes, &c.DraftRevision, &c.PublishedRevision, &c.UpdatedAt, &c.PublishedAt,
		)
	})
	if err != nil {
		return ThemeConfiguration{}, err
	}
	_ = json.Unmarshal(draftBytes, &c.DraftConfig)
	_ = json.Unmarshal(publishedBytes, &c.PublishedConfig)
	return c, nil
}

func (r Repository) DiscardDraft(ctx context.Context, installationID string) (int, error) {
	var rev int
	err := r.pool.QueryRow(ctx, `
		UPDATE theme_configurations
		SET draft_config = published_config, draft_revision = draft_revision + 1, updated_at = now()
		WHERE installation_id = $1
		RETURNING draft_revision
	`, installationID).Scan(&rev)
	if err != nil {
		return 0, translatePGError(err, "discard draft")
	}
	return rev, nil
}

// --- Assets ---

func (r Repository) CreateAsset(ctx context.Context, themeVersionID, assetType, uri, integrity string, metadata map[string]any) (ThemeAsset, error) {
	if themeVersionID == "" || assetType == "" || uri == "" {
		return ThemeAsset{}, ErrInvalidInput
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	var a ThemeAsset
	id := uuid.NewString()
	metaBytes, _ := json.Marshal(metadata)
	err := r.pool.QueryRow(ctx, `
		INSERT INTO theme_assets (id, theme_version_id, asset_type, uri, integrity, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at
	`, id, themeVersionID, assetType, uri, integrity, metaBytes).Scan(&a.CreatedAt)
	if err != nil {
		return ThemeAsset{}, translatePGError(err, "create asset")
	}
	a.ID = id
	a.ThemeVersionID = themeVersionID
	a.AssetType = assetType
	a.URI = uri
	a.Integrity = integrity
	a.Metadata = metadata
	return a, nil
}

func (r Repository) ListAssets(ctx context.Context, themeVersionID string) ([]ThemeAsset, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, theme_version_id, asset_type, uri, integrity, metadata, created_at
		FROM theme_assets WHERE theme_version_id = $1 ORDER BY created_at ASC, id ASC
	`, themeVersionID)
	if err != nil {
		return nil, fmt.Errorf("list assets: %w", err)
	}
	defer rows.Close()
	var out []ThemeAsset
	for rows.Next() {
		var a ThemeAsset
		var metaBytes []byte
		if err := rows.Scan(&a.ID, &a.ThemeVersionID, &a.AssetType, &a.URI, &a.Integrity, &metaBytes, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan asset: %w", err)
		}
		_ = json.Unmarshal(metaBytes, &a.Metadata)
		out = append(out, a)
	}
	return out, rows.Err()
}
