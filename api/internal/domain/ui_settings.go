package domain

// UISettings is one client UI-state blob, stored verbatim under a scope key.
//
// The value is OPAQUE to the daemon: it is whatever JSON object the client PUT,
// compacted and kept as text. The daemon never parses, merges, migrates or
// validates its shape, because the shape is a client layout decision and
// modelling it server-side would couple the daemon to it. Scope is the only
// thing the daemon understands — "global" for machine-wide preferences, or
// "workspace:<workspaceId>" for the per-workspace stores.
//
// It lives as a row in the global state/view.db beside terminal profiles and
// agent provider preferences, so a client with no local persistence (the
// Rust-native desktop app) recovers its full UI state from the daemon on boot.
type UISettings struct {
	Scope string `gorm:"primaryKey" json:"scope"`
	Value string `json:"value"`
}

// TableName pins the sqlite table the generic store auto-migrates and reads,
// independent of struct-name pluralisation.
func (UISettings) TableName() string { return "ui_settings" }
