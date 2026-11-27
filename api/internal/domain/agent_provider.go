package domain

type AgentProvider struct {
	ID          string
	DisplayName string
	Icon        string
	Connected   bool
	Enabled     bool
	MCPEnabled  bool

	SignIn string

	Compaction bool

	ShellStop bool

	Hotswap     bool
	PromptSteer bool
	HasTerminal bool

	TerminalStartHere bool

	ModelSelect  bool
	EffortSelect bool

	ActivityEvents []string

	Models  []string
	Efforts map[string][]string

	DefaultModel string

	DefaultEfforts map[string]string

	PermissionLevels []string
}
