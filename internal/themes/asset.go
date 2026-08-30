package themes

import "time"

// ThemeAsset is metadata for a theme asset (image, font, etc.) served through
// object storage/CDN. Large binaries are never stored in PostgreSQL; only
// metadata and an integrity hash live here. Seller-uploaded theme code is not
// permitted.
type ThemeAsset struct {
	ID                   string         `json:"id"`
	ThemeVersionID       string         `json:"theme_version_id"`
	AssetType            string         `json:"asset_type"`
	URI                  string         `json:"uri"`
	Integrity            string         `json:"integrity"`
	Metadata             map[string]any `json:"metadata"`
	CreatedAt            time.Time      `json:"created_at"`
}
