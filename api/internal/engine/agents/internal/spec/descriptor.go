package spec

type Descriptor struct {
	ID            string `yaml:"id"`
	SchemaVersion int    `yaml:"schema_version"`

	ProtocolVersion *VersionRange       `yaml:"protocol_version"`
	Runtime         RuntimeSpec         `yaml:"runtime"`
	Events          EventTable          `yaml:"events"`
	Catalog         map[string]CallSpec `yaml:"catalog"`
	Inject          []InjectSpec        `yaml:"inject"`

	DisplayName string `yaml:"display_name"`
	Icon        string `yaml:"icon"`
	SignIn      string `yaml:"sign_in"`

	Spawn   SpawnSpec   `yaml:"spawn"`
	Session SessionSpec `yaml:"session"`

	ConfigInjection InjectSteps `yaml:"config_injection"`

	HooksInjection InjectSteps `yaml:"hooks_injection"`

	MCPInject InjectSteps `yaml:"mcp_injection"`

	AddDirs InjectSteps `yaml:"add_dirs"`

	ContextInject InjectSteps `yaml:"context_inject"`

	ResumeContextInject InjectSteps `yaml:"resume_context_inject"`

	Presentation PresentationSpec `yaml:"presentation"`

	Model  *ModelSpec  `yaml:"model"`
	Effort *EffortSpec `yaml:"effort"`

	PermissionLevels *PermissionSpec `yaml:"permission_levels"`

	Telemetry *TelemetrySpec `yaml:"telemetry"`

	TerminalPrompts []TerminalPromptSpec `yaml:"terminal_prompts"`

	TerminalNotices []TerminalNoticeSpec `yaml:"terminal_notices"`

	InjectedPrompts []InjectedPromptSpec `yaml:"injected_prompts"`

	NotEmitted []string `yaml:"not_emitted"`

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
	Resume   *ArgSpec            `yaml:"resume"`
	Assign   *ArgSpec            `yaml:"assign"`
	Refused  *SessionRefusedSpec `yaml:"refused"`
	Persists string              `yaml:"persists"`
}

type SessionRefusedSpec struct {
	ExitCode int      `yaml:"exit_code"`
	RPCCode  int      `yaml:"rpc_code"`
	Needles  []string `yaml:"needles"`
}

type ArgSpec struct {
	Arg string `yaml:"arg"`
}
