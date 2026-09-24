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

	ConfigInjection []InjectStep `yaml:"config_injection"`

	// HooksInjection wires the provider's hook relay (`crowbar hook <event>`).
	// It is applied ONLY to a process whose delivery channel is hooks — a PTY
	// — never to an api-transport `serve` process: a process reports over
	// exactly one channel, so an event can never arrive twice (sessions spec
	// §2.3).
	HooksInjection []InjectStep `yaml:"hooks_injection"`

	MCPInject []InjectStep `yaml:"mcp_injection"`

	ContextInject []InjectStep `yaml:"context_inject"`

	ResumeContextInject []InjectStep `yaml:"resume_context_inject"`

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

	// NotEmitted names canonical in/ask events this provider genuinely never
	// fires — an absent event otherwise means both "the provider doesn't do
	// this" and "we forgot", indistinguishably. Enforced (rules.notEmitted):
	// every in/ask canonical event must be mapped in events: or listed here
	// (design spec 2.6).
	NotEmitted []string `yaml:"not_emitted"`

	// Surfaces declares which VIEWS this provider offers (chat, terminal) and
	// which may be launched a brand-new chat directly onto (design spec 2.5).
	// Keyed by surface name; validated by rules.surfaces against RuntimeSpec/
	// Hotswap so it cannot silently contradict them. Nil for a descriptor
	// that has not declared any — every surface then answers false to
	// SurfaceStartHere, same as every other absent capability.
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
}

type ArgSpec struct {
	Arg string `yaml:"arg"`
}
