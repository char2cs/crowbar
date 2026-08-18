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

	// Model and Effort declare what a chat may choose to run this CLI as, and how
	// the choice reaches the process. Both are POINTERS because absent means the
	// capability does not exist for this provider — not that it exists and is
	// empty — and the two render differently: no picker at all, rather than a
	// picker offering nothing.
	//
	// They are separate types rather than one parameterised block because their
	// catalogues genuinely differ in shape: an effort level belongs to a model,
	// and a model belongs to nothing.
	Model  *ModelSpec  `yaml:"model"`
	Effort *EffortSpec `yaml:"effort"`

	// Telemetry declares how (and whether) this provider reports context usage,
	// cost, rate limits and resolved model identity. Every fact is independently
	// optional; see TelemetrySpec.
	Telemetry *TelemetrySpec `yaml:"telemetry"`

	// Answer is the write side of Hooks: how a decision a human took in Crowbar
	// reaches the CLI blocked waiting for it. Absent means every prompt this
	// provider opens is observe-only, and the relay never holds a hook open.
	Answer AnswerSpec `yaml:"answer"`

	// TerminalPrompts is the COMPLEMENT of Answer: the prompts this CLI opens
	// that no hook reports and Crowbar therefore cannot answer at all. Declaring
	// them is what lets a chat say "your agent is waiting in the terminal"
	// instead of rendering an empty pane over a blocked process.
	//
	// Empty is the safe default and the honest one for a provider nobody has
	// captured a screen from: it never matches, so nothing about that provider's
	// chats changes.
	TerminalPrompts []TerminalPromptSpec `yaml:"terminal_prompts"`
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
