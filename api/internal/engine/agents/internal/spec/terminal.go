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

	// Answers are the replies a client may offer for this modal, each the
	// keystrokes the CLI expects on its PTY.
	Answers []TerminalPromptAnswerSpec `yaml:"answers"`
}

type TerminalPromptAnswerSpec struct {
	ID string `yaml:"id"`

	Label string `yaml:"label"`

	Keys string `yaml:"keys"`
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
