package domain

// AgentProviderPreference is a global user preference for one agent provider: its
// position in the priority order (lower = higher priority), whether it is
// disabled, and whether Crowbar's own tool surface is registered with it.
// Persisted as a row keyed by provider id; a provider with no row uses defaults
// (enabled, MCP on, ordered after every preferenced provider by descriptor id).
//
// Both flags are stored NEGATIVELY, and that is load-bearing rather than a
// naming choice: the store auto-migrates this struct, so adding a column
// backfills every existing row with the zero value. A positive MCPEnabled would
// have switched the tool surface OFF for every provider the user had ever
// touched, the first time the daemon came up on the new schema. The wire flips
// both back to positive (dto.AgentProviderDTO) because a UI switch reads as "on".
type AgentProviderPreference struct {
	ProviderID  string `gorm:"primaryKey" json:"providerId"`
	Priority    int    `json:"priority"`
	Disabled    bool   `json:"disabled"`
	MCPDisabled bool   `json:"mcpDisabled"`
}

// TableName pins the sqlite table the generic store auto-migrates and reads,
// independent of struct-name pluralisation.
func (AgentProviderPreference) TableName() string { return "agent_provider_preferences" }
