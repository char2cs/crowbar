package domain

type AgentProviderPreference struct {
	ProviderID  string `gorm:"primaryKey" json:"providerId"`
	Priority    int    `json:"priority"`
	Disabled    bool   `json:"disabled"`
	MCPDisabled bool   `json:"mcpDisabled"`
}

func (AgentProviderPreference) TableName() string { return "agent_provider_preferences" }
