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

// AgentProviderDTO is the wire shape of domain.AgentProvider (see there for the field
// semantics). Models and Efforts are omitted rather than empty for a provider with no
// catalogue: absence says the picker does not exist.
type AgentProviderDTO struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Icon        string `json:"icon"`
	Connected   bool   `json:"connected"`
	Enabled     bool   `json:"enabled"`
	MCPEnabled  bool   `json:"mcpEnabled"`
	Compaction  bool   `json:"compaction"`
	Hotswap     bool   `json:"hotswap"`
	// SignIn is the shell command that logs the CLI in, omitted when undeclared.
	SignIn string `json:"signIn,omitempty"`
	// PromptSteer says a message sent while a turn runs is delivered into that
	// turn, so the composer need not hold it until the turn ends.
	PromptSteer bool `json:"promptSteer"`
	HasTerminal bool `json:"hasTerminal"`
	// TerminalStartHere says a new chat may launch directly onto this provider's
	// terminal surface; it implies HasTerminal but not the reverse.
	TerminalStartHere bool     `json:"terminalStartHere"`
	ModelSelect       bool     `json:"modelSelect"`
	EffortSelect      bool     `json:"effortSelect"`
	ActivityEvents    []string `json:"activityEvents,omitempty"`
	Models            []string `json:"models,omitempty"`
	// DefaultModel is the provider's own stated default, omitted when unknown so a
	// client can't misread "" as a fact.
	DefaultModel string              `json:"defaultModel,omitempty"`
	Efforts      map[string][]string `json:"efforts,omitempty"`
	// DefaultEfforts is each model's stated default effort, absent for a model stating none.
	DefaultEfforts map[string]string `json:"defaultEfforts,omitempty"`

	// PermissionLevels mirrors Models/Efforts' own "omitted, not empty" rule:
	// absent says this provider's picker offers nothing, where an empty array
	// would not. See domain.AgentProvider.PermissionLevels.
	PermissionLevels []string `json:"permissionLevels,omitempty"`
}
