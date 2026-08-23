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
}

const (
	TerminalNoticeUsageLimit = "usage_limit"
)

var TerminalNoticeKinds = map[string]struct{}{
	TerminalNoticeUsageLimit: {},
}

type TerminalNoticeSpec struct {
	Kind string `yaml:"kind"`

	Needle string `yaml:"needle"`

	EndsTurn bool `yaml:"ends_turn"`
}
