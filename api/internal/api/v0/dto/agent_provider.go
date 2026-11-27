package dto

type SlashCatalogDTO struct {
	ProviderID   string                `json:"providerId"`
	Completeness string                `json:"completeness"`
	Items        []SlashCatalogItemDTO `json:"items"`
	Warnings     []string              `json:"warnings"`
}

type SlashCatalogItemDTO struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Description string `json:"description"`
	InsertText  string `json:"insertText"`
	Source      string `json:"source"`
	Terminal    bool   `json:"terminal,omitempty"`
}

type AgentProviderDTO struct {
	ID                string              `json:"id"`
	DisplayName       string              `json:"displayName"`
	Icon              string              `json:"icon"`
	Connected         bool                `json:"connected"`
	Enabled           bool                `json:"enabled"`
	MCPEnabled        bool                `json:"mcpEnabled"`
	Compaction        bool                `json:"compaction"`
	ShellStop         bool                `json:"shellStop"`
	Hotswap           bool                `json:"hotswap"`
	SignIn            string              `json:"signIn,omitempty"`
	PromptSteer       bool                `json:"promptSteer"`
	HasTerminal       bool                `json:"hasTerminal"`
	TerminalStartHere bool                `json:"terminalStartHere"`
	ModelSelect       bool                `json:"modelSelect"`
	EffortSelect      bool                `json:"effortSelect"`
	ActivityEvents    []string            `json:"activityEvents,omitempty"`
	Models            []string            `json:"models,omitempty"`
	DefaultModel      string              `json:"defaultModel,omitempty"`
	Efforts           map[string][]string `json:"efforts,omitempty"`
	DefaultEfforts    map[string]string   `json:"defaultEfforts,omitempty"`

	PermissionLevels []string `json:"permissionLevels,omitempty"`
}
