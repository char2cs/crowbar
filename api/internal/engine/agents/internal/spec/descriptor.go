package spec

type Descriptor struct {
	ID string `yaml:"id"`

	DisplayName string `yaml:"display_name"`
	Icon        string `yaml:"icon"`

	Spawn   SpawnSpec   `yaml:"spawn"`
	Session SessionSpec `yaml:"session"`

	ConfigInjection []InjectStep `yaml:"config_injection"`

	MCPInject []InjectStep `yaml:"mcp_injection"`

	Hooks HookSpec `yaml:"hooks"`

	ContextInject []InjectStep `yaml:"context_inject"`

	ResumeContextInject []InjectStep `yaml:"resume_context_inject"`

	Presentation PresentationSpec `yaml:"presentation"`

	Model  *ModelSpec  `yaml:"model"`
	Effort *EffortSpec `yaml:"effort"`

	Telemetry *TelemetrySpec `yaml:"telemetry"`

	Answer AnswerSpec `yaml:"answer"`

	TerminalPrompts []TerminalPromptSpec `yaml:"terminal_prompts"`

	TerminalNotices []TerminalNoticeSpec `yaml:"terminal_notices"`

	InjectedPrompts []InjectedPromptSpec `yaml:"injected_prompts"`
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
