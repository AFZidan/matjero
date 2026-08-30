package themes

import "time"

// ThemeVersionStatus enumerates the lifecycle state of a theme version.
const (
	ThemeVersionStatusDraft      = "draft"
	ThemeVersionStatusPublished  = "published"
	ThemeVersionStatusDeprecated = "deprecated"
)

// ThemeVersion is an immutable-after-publication release of a Theme. It carries
// the validated configuration JSON Schema and the deterministic default
// configuration. Store installations reference a specific Theme Version so that
// deployed presentation never mutates unpredictably for all stores.
//
// Immutability invariant: once a version is Published it must not be mutated
// in place. A change requires publishing a new version (v1 -> v2).
type ThemeVersion struct {
	ID                       string         `json:"id"`
	ThemeID                  string         `json:"theme_id"`
	Version                  string         `json:"version"`
	Status                   string         `json:"status"`
	ConfigurationSchema      map[string]any `json:"configuration_schema"`
	DefaultConfiguration     map[string]any `json:"default_configuration"`
	ComponentRegistryVersion string         `json:"component_registry_version"`
	CreatedAt                time.Time      `json:"created_at"`
	PublishedAt              *time.Time     `json:"published_at,omitempty"`
	DeprecatedAt             *time.Time     `json:"deprecated_at,omitempty"`
}

// ValidThemeVersionStatus reports whether s is a recognized theme version status.
func ValidThemeVersionStatus(s string) bool {
	switch s {
	case ThemeVersionStatusDraft, ThemeVersionStatusPublished, ThemeVersionStatusDeprecated:
		return true
	default:
		return false
	}
}
