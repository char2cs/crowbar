package dto

// SlashCatalogDTO is one ephemeral deterministic provider capability response.
// Completeness is never inferred by Crowbar; it is declared by the provider
// descriptor so partial inventories remain visibly partial.
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
}

// AgentProviderDTO is the wire shape of domain.AgentProvider (00 agentic-engine
// spec §7.2) — see there for what connected, enabled and mcpEnabled each mean,
// what orders the list, and why efforts arrives already resolved. The catalog is
// workspace-independent but served on the workspace-scoped route for surface
// consistency.
//
// models and efforts are OMITTED rather than sent empty for a provider that
// declares no catalogue: an absent field says the picker does not exist, which an
// empty array would not.
type AgentProviderDTO struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Icon        string `json:"icon"`
	Connected   bool   `json:"connected"`
	Enabled     bool   `json:"enabled"`
	MCPEnabled  bool   `json:"mcpEnabled"`
	Compaction  bool   `json:"compaction"`
	Hotswap     bool   `json:"hotswap"`
	// PromptSteer says a message sent while a turn runs is delivered into that
	// turn, so the composer need not hold it until the turn ends.
	PromptSteer bool `json:"promptSteer"`
	HasTerminal bool `json:"hasTerminal"`
	// TerminalStartHere is domain.AgentProvider.TerminalStartHere — whether a
	// brand-new chat may be launched DIRECTLY onto this provider's terminal
	// surface (design spec 2.5). False whenever HasTerminal is false, and may
	// be false even when it is true.
	TerminalStartHere bool     `json:"terminalStartHere"`
	ModelSelect       bool     `json:"modelSelect"`
	EffortSelect      bool     `json:"effortSelect"`
	ActivityEvents    []string `json:"activityEvents,omitempty"`
	Models            []string `json:"models,omitempty"`
	// DefaultModel is domain.AgentProvider.DefaultModel: the provider's OWN
	// stated default, omitted (never sent as "") when unknown — an absent
	// field says "the provider didn't say", where "" could misread as "no
	// default model" as a fact rather than as missing information.
	DefaultModel string              `json:"defaultModel,omitempty"`
	Efforts      map[string][]string `json:"efforts,omitempty"`

	// PermissionLevels mirrors Models/Efforts' own "omitted, not empty" rule:
	// absent says this provider's picker offers nothing, where an empty array
	// would not. See domain.AgentProvider.PermissionLevels.
	PermissionLevels []string `json:"permissionLevels,omitempty"`
}
