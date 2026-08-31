package themes

import "time"

// ThemeInstallationStatus enumerates the selection state of an installation.
const (
	ThemeInstallationStatusActive   = "active"
	ThemeInstallationStatusInactive = "inactive"
)

// ThemeInstallation binds a Store to a specific Theme Version. A store has at
// most one active installation (enforced by a partial unique index in
// PostgreSQL). Inactive installations remain as history when a seller switches
// or upgrades themes. Switching or upgrading a theme never mutates commerce data.
type ThemeInstallation struct {
	ID             string    `json:"id"`
	StoreID        string    `json:"store_id"`
	ThemeID        string    `json:"theme_id"`
	ThemeVersionID string    `json:"theme_version_id"`
	Status         string    `json:"status"`
	InstalledAt    time.Time `json:"installed_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ValidThemeInstallationStatus reports whether s is a recognized installation status.
func ValidThemeInstallationStatus(s string) bool {
	return s == ThemeInstallationStatusActive || s == ThemeInstallationStatusInactive
}
