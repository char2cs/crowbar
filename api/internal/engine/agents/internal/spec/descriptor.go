package spec

// Descriptor holds only fields the engine consumes. Every field here is
// load-bearing; provider-specific shapes (hook-config layout, native event
// names) live in the descriptor's literal write-side content, never here.
type Descriptor struct {
	ID string `yaml:"id"`
	// DisplayName and Icon are the ONLY display-only fields (the "every field is
	// load-bearing" invariant's documented carve-out): they label and glyph the
	// agent in the UI and never influence spawn/hook behaviour. Both optional.
	DisplayName string `yaml:"display_name"`
	Icon        string `yaml:"icon"`

	Spawn   SpawnSpec   `yaml:"spawn"`
	Session SessionSpec `yaml:"session"`

	ConfigInjection []InjectStep `yaml:"config_injection"`

	// MCPInject registers Crowbar's own tool surface with this CLI, and is a
	// SEPARATE list from ConfigInjection because it is the one group of steps
	// that is conditional: the user can switch the tool surface off per provider
	// while the CLI still spawns and its hooks still fire.
	//
	// Naming the group is the only mechanism that can express that. Filtering
	// ConfigInjection by template token was the obvious alternative and is wrong:
	// {runner_token} appears in exactly one of codex's four MCP steps, so the
	// other three would survive the filter and register a server with no
	// arguments — a half-configured tool surface, worse than either state the
	// switch is meant to choose between.
	MCPInject []InjectStep `yaml:"mcp_injection"`

	Hooks HookSpec `yaml:"hooks"`

	// ContextInject delivers Crowbar's {context} document to a CLI starting a
	// FRESH provider session, through a channel the model reads WITHOUT being
	// given a turn (claude's --append-system-prompt, codex's
	// `-c developer_instructions=`). A positional prompt is NOT such a channel —
	// it IS the user's first message, so the CLI would answer the handoff
	// instead of waiting for the user.
	ContextInject []InjectStep `yaml:"context_inject"`

	// ResumeContextInject delivers {context} to a CLI resumed into its OWN prior
	// native session, where {context} is only the GAP.
	//
	// Separate from ContextInject because a resumed session does not necessarily
	// accept the same channel a fresh one does. Verified against codex 0.139.0:
	// `codex resume` rebuilds from its rollout file, which never records
	// developer instructions, so a new `-c developer_instructions=` is silently
	// ignored. The only channel that reaches a resumed codex is a user message.
	ResumeContextInject []InjectStep `yaml:"resume_context_inject"`

	// Presentation contains optional, declarative adapters for the chat
	// presentation. It never changes whether the provider runs in its native
	// terminal: an absent capability means terminal-only for that operation.
	Presentation PresentationSpec `yaml:"presentation"`

	// Telemetry declares how (and whether) this provider reports context usage,
	// cost, rate limits and resolved model identity. Every fact is independently
	// optional; see TelemetrySpec.
	Telemetry *TelemetrySpec `yaml:"telemetry"`
}

type SpawnSpec struct {
	Cmd                 string   `yaml:"cmd"`
	InteractiveRequired bool     `yaml:"interactive_required"`
	ForbidFlags         []string `yaml:"forbid_flags"`
	Args                []string `yaml:"args"`
	Env                 struct {
		Clear []string `yaml:"clear"`
	} `yaml:"env"`
}

type SessionSpec struct {
	Resume *ArgSpec `yaml:"resume"`
}

type ArgSpec struct {
	Arg string `yaml:"arg"`
}
