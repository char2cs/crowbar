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

	// Cursor marks the selected row of a selection modal; Down and Up move it.
	// An answer naming an Option is aimed from the cursor's row on the screen.
	Cursor string `yaml:"cursor"`
	Down   string `yaml:"down"`
	Up     string `yaml:"up"`
}

type TerminalPromptAnswerSpec struct {
	ID string `yaml:"id"`

	Label string `yaml:"label"`

	Keys string `yaml:"keys"`

	// Option is the row's text this answer selects before typing Keys: a CLI
	// may preselect any row, so the answer aims by text, never by position.
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

	// Anchor opens the banner's raw row, before Needle: what tells the
	// provider's banner from a reply or a prompt echo quoting its sentence.
	Anchor string `yaml:"anchor"`

	Needle string `yaml:"needle"`

	EndsTurn bool `yaml:"ends_turn"`
}
