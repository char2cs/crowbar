package domain

import "slices"

type AgentTerminalWait struct {
	Waiting bool

	Kind string

	Answers []AgentTerminalAnswer
}

type AgentTerminalAnswer struct {
	ID string

	Label string

	Keys string
}

func (w AgentTerminalWait) Equal(o AgentTerminalWait) bool {
	return w.Waiting == o.Waiting && w.Kind == o.Kind && slices.Equal(w.Answers, o.Answers)
}

const (
	AgentTerminalWaitTrust = "workspace_trust"
)
