package themes

import "time"

// ThemeConfiguration holds the draft and published configuration for a single
// Theme Installation, plus revision counters used for deterministic,
// revision-based cache keys (e.g. store:{store_id}:theme:{published_revision}).
//
// The public storefront consumes only PublishedConfig. Seller editing mutates
// DraftConfig. Publishing atomically copies DraftConfig into PublishedConfig and
// bumps PublishedRevision. Draft edits bump DraftRevision without disturbing the
// live storefront.
type ThemeConfiguration struct {
	ID                string         `json:"id"`
	InstallationID    string         `json:"installation_id"`
	DraftConfig       map[string]any `json:"draft_config"`
	PublishedConfig   map[string]any `json:"published_config"`
	DraftRevision     int            `json:"draft_revision"`
	PublishedRevision int            `json:"published_revision"`
	UpdatedAt         time.Time      `json:"updated_at"`
	PublishedAt       *time.Time     `json:"published_at,omitempty"`
}
