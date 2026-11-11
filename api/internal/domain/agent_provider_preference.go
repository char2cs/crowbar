package domain

// AgentProviderPreference is a global preference for one agent provider: priority
// (lower first) and the disabled/MCP flags; a provider with no row uses defaults. The
// flags are stored negatively so auto-migration's zero-value backfill means "on".
type AgentProviderPreference struct {
	ProviderID  string `gorm:"primaryKey" json:"providerId"`
	Priority    int    `json:"priority"`
	Disabled    bool   `json:"disabled"`
	MCPDisabled bool   `json:"mcpDisabled"`
}

// TableName pins the sqlite table the generic store auto-migrates and reads,
// independent of struct-name pluralisation.
func (AgentProviderPreference) TableName() string { return "agent_provider_preferences" }
