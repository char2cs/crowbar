package domain

// AgentProviderPreference is a global user preference for one agent provider: its
// position in the priority order (lower = higher priority) and whether it is
// disabled. Persisted as a row keyed by provider id; a provider with no row uses
// defaults (enabled, ordered after every preferenced provider by descriptor id).
type AgentProviderPreference struct {
	ProviderID string `gorm:"primaryKey" json:"providerId"`
	Priority   int    `json:"priority"`
	Disabled   bool   `json:"disabled"`
}

// TableName pins the sqlite table the generic store auto-migrates and reads,
// independent of struct-name pluralisation.
func (AgentProviderPreference) TableName() string { return "agent_provider_preferences" }
