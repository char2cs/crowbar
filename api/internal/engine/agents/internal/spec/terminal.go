package spec

const (
	TerminalPromptTrust = "workspace_trust"
)

var TerminalPromptKinds = map[string]struct{}{
	TerminalPromptTrust: {},
}

type TerminalPromptSpec struct {
	Kind string `yaml:"kind"`

	Needle string `yaml:"needle"`

	Answers []TerminalPromptAnswerSpec `yaml:"answers"`

	Cursor string `yaml:"cursor"`
	Down   string `yaml:"down"`
	Up     string `yaml:"up"`
}

type TerminalPromptAnswerSpec struct {
	ID string `yaml:"id"`

	Label string `yaml:"label"`

	Keys string `yaml:"keys"`

	Option string `yaml:"option"`
}

const (
	TerminalNoticeUsageLimit = "usage_limit"
)

var TerminalNoticeKinds = map[string]struct{}{
	TerminalNoticeUsageLimit: {},
}

type TerminalNoticeSpec struct {
	Kind string `yaml:"kind"`

	Anchor string `yaml:"anchor"`

	Needle string `yaml:"needle"`

	EndsTurn bool `yaml:"ends_turn"`
}
