package domain

// AgentProvider is one provider as resolved from descriptors, global preferences and the install
// probe; ordered by priority (slice position). Connected means spawn.cmd is on PATH. MCPEnabled is
// separate from Enabled: tools off still spawns and chats. Both read negatively stored flags.
type AgentProvider struct {
	ID          string
	DisplayName string
	Icon        string
	Connected   bool
	Enabled     bool
	MCPEnabled  bool

	// SignIn is the descriptor's command that logs the CLI in; "" when it declares none.
	SignIn string

	// Compaction is whether the provider declares a gesture to compact its own context; a compact
	// control must be gated on it, since a button that does nothing is worse than none.
	Compaction bool

	// Hotswap and HasTerminal are engine.Capabilities.Hotswap/.HasTerminal,
	// carried through unchanged — see there for what each means.
	Hotswap bool
	// PromptSteer is engine.Capabilities.PromptSteer: a message sent mid-turn is
	// accepted and delivered into the running turn, so a client need not wait.
	PromptSteer bool
	HasTerminal bool

	// TerminalStartHere is engine.Capabilities.TerminalStartHere: whether a new chat may launch directly
	// onto the terminal surface rather than switching there after spawn. Implies HasTerminal.
	TerminalStartHere bool

	// ModelSelect and EffortSelect are whether the descriptor declares a model / effort catalogue;
	// false means no picker at all, never a disabled control implying breakage.
	ModelSelect  bool
	EffortSelect bool

	// ActivityEvents is the provider-neutral canonical event vocabulary this
	// provider's descriptor actually observes. The UI can gate rich surfaces on
	// these facts without inspecting a provider id.
	ActivityEvents []string

	// Models is the declared catalogue in descriptor order. Efforts is keyed by model id (plus "" for
	// the default model) and already resolved, so clients need no fallback rule. Both are nil, not
	// empty, when undeclared: an omitted field says "no picker" where an empty array would not.
	Models  []string
	Efforts map[string][]string

	// DefaultModel is the default the descriptor (or, for discovered models, the live source) states;
	// empty means unknown, never a cue to fall back to Models[0].
	DefaultModel string

	// PermissionLevels lists which permission levels the descriptor declares, so a client offers only
	// what SetChatPermissionLevel would accept (absent, never clamped). Nil when none.
	PermissionLevels []string
}
