// Package themes implements the reusable, versioned, schema-driven Theme Engine
// used by the native storefront. It is intentionally decoupled from commerce core
// (pricing, inventory, orders, catalog ownership): a theme consumes storefront
// data for presentation but never mutates commerce source-of-truth.
package themes

import "time"

// ThemeType enumerates the commercial class of a theme. Phase 4 ships only
// platform-controlled FREE themes, but the model already supports PREMIUM for a
// future marketplace without changing the schema.
const (
	ThemeTypeFree    = "free"
	ThemeTypePremium = "premium"
)

// ThemeStatus enumerates the lifecycle state of a theme definition.
const (
	ThemeStatusDraft     = "draft"
	ThemeStatusActive    = "active"
	ThemeStatusDeprecated = "deprecated"
	ThemeStatusDisabled   = "disabled"
)

// Theme is a platform-controlled theme definition. Its Key is the stable, unique
// identifier used by the registry and storefront resolution; it never changes
// even when versions are published.
type Theme struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Type        string    `json:"type"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ValidThemeType reports whether t is a recognized theme type.
func ValidThemeType(t string) bool {
	return t == ThemeTypeFree || t == ThemeTypePremium
}

// ValidThemeStatus reports whether s is a recognized theme status.
func ValidThemeStatus(s string) bool {
	switch s {
	case ThemeStatusDraft, ThemeStatusActive, ThemeStatusDeprecated, ThemeStatusDisabled:
		return true
	default:
		return false
	}
}
