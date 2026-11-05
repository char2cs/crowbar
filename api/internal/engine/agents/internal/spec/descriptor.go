package spec

type Descriptor struct {
	ID string `yaml:"id"`

	// --- v3: the event is the unit ------------------------------------------
	ProtocolVersion *VersionRange       `yaml:"protocol_version"`
	Runtime         RuntimeSpec         `yaml:"runtime"`
	Events          EventTable          `yaml:"events"`
	Catalog         map[string]CallSpec `yaml:"catalog"`
	Inject          []InjectSpec        `yaml:"inject"`

	DisplayName string `yaml:"display_name"`
	Icon        string `yaml:"icon"`

	Spawn   SpawnSpec   `yaml:"spawn"`
	Session SessionSpec `yaml:"session"`

	ConfigInjection InjectSteps `yaml:"config_injection"`

	// HooksInjection points the provider's hooks at the daemon's hook server. It is
	// applied ONLY to a hooks-channel PTY, never an api `serve` process: each process
	// reports over exactly one channel, so an event never arrives twice.
	HooksInjection InjectSteps `yaml:"hooks_injection"`

	MCPInject InjectSteps `yaml:"mcp_injection"`

	ContextInject InjectSteps `yaml:"context_inject"`

	ResumeContextInject InjectSteps `yaml:"resume_context_inject"`

	Presentation PresentationSpec `yaml:"presentation"`

	Model  *ModelSpec  `yaml:"model"`
	Effort *EffortSpec `yaml:"effort"`

	// PermissionLevels is separate from the top-level "permission" event key
	// (events.permission, the PermissionRequest hook mapping) — this one
	// declares spawn-time behavior, that one declares an in-conversation ask.
	PermissionLevels *PermissionSpec `yaml:"permission_levels"`

	Telemetry *TelemetrySpec `yaml:"telemetry"`

	TerminalPrompts []TerminalPromptSpec `yaml:"terminal_prompts"`

	TerminalNotices []TerminalNoticeSpec `yaml:"terminal_notices"`

	InjectedPrompts []InjectedPromptSpec `yaml:"injected_prompts"`

	// NotEmitted names canonical in/ask events this provider never fires (or that
	// mapping would misread). rules.notEmitted requires every in/ask event be mapped
	// or listed here, so "doesn't do this" is distinguishable from "forgot".
	NotEmitted []string `yaml:"not_emitted"`

	// Surfaces declares which views (chat, terminal) this provider offers and which a
	// new chat may launch onto, keyed by surface name and validated by rules.surfaces
	// against the runtime. Nil means every surface answers false to SurfaceStartHere.
	Surfaces map[string]SurfaceSpec `yaml:"surfaces"`
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
	// Assign launches the CLI on a session id Crowbar minted.
	Assign *ArgSpec `yaml:"assign"`
	// Refused is how the CLI says it cannot resume a session: the only
	// evidence that a chat's session is lost.
	Refused *SessionRefusedSpec `yaml:"refused"`
	// Persists is when the CLI has written a session it can resume:
	// "turn_start" (the default) or "turn_end", once a turn on it completes.
	Persists string `yaml:"persists"`
}

// SessionRefusedSpec is a refused resume, by one rule per channel: on the
// terminal the CLI exits with ExitCode having printed a line containing one of
// Needles; over the api a call fails with RPCCode and a message containing one.
type SessionRefusedSpec struct {
	ExitCode int      `yaml:"exit_code"`
	RPCCode  int      `yaml:"rpc_code"`
	Needles  []string `yaml:"needles"`
}

type ArgSpec struct {
	Arg string `yaml:"arg"`
}
