package domain

import "slices"

type AgentTerminalWait struct {
	Waiting bool

	Kind string

	// Answers are the replies the provider's descriptor declares for this prompt.
	Answers []AgentTerminalAnswer
}

// AgentTerminalAnswer is one reply to a terminal prompt. Keys are the PTY
// keystrokes that deliver it and never leave the daemon.
type AgentTerminalAnswer struct {
	ID string

	Label string

	Keys string
}

// Equal reports whether two verdicts would render identically.
func (w AgentTerminalWait) Equal(o AgentTerminalWait) bool {
	return w.Waiting == o.Waiting && w.Kind == o.Kind && slices.Equal(w.Answers, o.Answers)
}

const (
	AgentTerminalWaitTrust = "workspace_trust"
)
